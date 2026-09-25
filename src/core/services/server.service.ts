import { createHash, timingSafeEqual } from 'node:crypto';
import { GhostwireError, toGhostwireError } from '../domain/errors.js';
import { decodeInputEvent } from '../domain/input/events.js';
import {
  FrameEncoding,
  MessageKind,
  PixelFormat,
  PROTOCOL_VERSION,
  SERVER_VERSION,
} from '../domain/protocol/constants.js';
import { MessageDecoder, decodeControlPayload, encodeJSONMessage, encodeMessage } from '../domain/protocol/codec.js';
import { decodeBye, decodeHello, decodePing, encodeError, encodeWelcome } from '../domain/protocol/messages.js';
import type { DecodedMessage } from '../domain/protocol/codec.js';
import { samePixels } from '../domain/screen/frame.js';
import type { ScreenFrame, ScreenSize } from '../domain/screen/frame.js';
import { encodeFrameMeta } from '../domain/protocol/frame.js';
import type { ServeOptions } from '../domain/session/config.js';
import type { ServerPort, ServerStats } from '../ports/inbound/server.js';
import type { ScreenCapturerPort } from '../ports/outbound/capture.js';
import type { CompressorPort } from '../ports/outbound/compress.js';
import type { InputInjectorPort } from '../ports/outbound/inject.js';
import type { Logger } from '../ports/outbound/logger.js';
import type { Conn, Listener, TransportPort } from '../ports/outbound/transport.js';

export interface ServerServiceDeps {
  transport: TransportPort;
  capturer: ScreenCapturerPort;
  injector: InputInjectorPort;
  compressor: CompressorPort;
  logger: Logger;
}

function tokensEqual(a: string, b: string): boolean {
  const hashA = createHash('sha256').update(a, 'utf8').digest();
  const hashB = createHash('sha256').update(b, 'utf8').digest();
  return timingSafeEqual(hashA, hashB);
}

interface HostState {
  options: ServeOptions;
  listener: Listener;
  active: ActiveSession | null;
  closed: boolean;
  captureTimer: NodeJS.Timeout | null;
}

interface ActiveSession {
  conn: Conn;
  decoder: MessageDecoder;
  authenticated: boolean;
  closed: boolean;
  lastSeenAt: number;
  handshakeTimer: NodeJS.Timeout | null;
  keepaliveTimer: NodeJS.Timeout | null;
  paused: boolean;
}

export class ServerService implements ServerPort {
  private readonly deps: ServerServiceDeps;
  private host: HostState | null = null;
  private readonly counters: ServerStats = {
    connectionsAccepted: 0,
    sessionsServed: 0,
    framesSent: 0,
    framesSkipped: 0,
    inputEvents: 0,
    bytesSent: 0,
  };

  constructor(deps: ServerServiceDeps) {
    this.deps = deps;
  }

  stats(): ServerStats {
    return { ...this.counters };
  }

  private grabFrame(): ScreenFrame | null {
    try {
      return this.deps.capturer.grab();
    } catch (err) {
      this.deps.logger.warn('capture failed', { error: String(err) });
      return null;
    }
  }

  async serve(options: ServeOptions, signal: AbortSignal): Promise<void> {
    if (this.host) {
      throw new GhostwireError('internal', 'server is already running');
    }
    const { transport, capturer, logger } = this.deps;
    if (!options.token) {
      throw new GhostwireError('auth', 'a token is required to start the server');
    }
    if (options.fps < 1 || options.fps > 60) {
      throw new GhostwireError('internal', 'fps must be between 1 and 60');
    }

    await capturer.start({ maxWidth: options.maxWidth });
    let listener: Listener;
    try {
      listener = await transport.listen(options.address, options.tls);
    } catch (err) {
      await capturer.close();
      throw toGhostwireError(err, 'transport');
    }

    const host: HostState = {
      options,
      listener,
      active: null,
      closed: false,
      captureTimer: null,
    };
    this.host = host;
    const screen = capturer.bounds();
    logger.info('server listening', {
      address: listener.address,
      screen: `${screen.width}x${screen.height}`,
      fps: options.fps,
      compress: options.compress,
      tls: options.tls ? 'enabled' : 'disabled',
    });

    let lastPixels: Uint8Array | null = null;
    let lastBounds: ScreenSize = screen;
    let seq = 0;

    const session = (): ActiveSession | null => {
      const active = host.active;
      if (!active || active.closed || !active.authenticated) return null;
      return active;
    };

    const broadcastScreenInfo = (size: ScreenSize): void => {
      const active = session();
      if (!active) return;
      active.conn.write(encodeJSONMessage(MessageKind.ScreenInfo, size));
    };

    const captureTick = async (): Promise<void> => {
      if (host.closed) return;
      const active = session();
      if (!active || active.paused) return;
      const frame = this.grabFrame();
      if (!frame) return;
      const bounds = this.deps.capturer.bounds();
      if (bounds.width !== lastBounds.width || bounds.height !== lastBounds.height) {
        lastBounds = { width: bounds.width, height: bounds.height };
        lastPixels = null;
        broadcastScreenInfo(lastBounds);
      }
      if (lastPixels && samePixels(frame.data, lastPixels)) {
        this.counters.framesSkipped++;
        return;
      }
      const rawLength = frame.data.length;
      let encoding: number = FrameEncoding.Raw;
      let body: Uint8Array = frame.data;
      if (this.deps.compressor.name === 'deflate') {
        try {
          body = await this.deps.compressor.compress(frame.data);
          encoding = FrameEncoding.Deflate;
        } catch (err) {
          logger.warn('frame compression failed', { error: String(err) });
          return;
        }
      }
      if (host.closed || session() !== active) return;
      const metaBytes = encodeFrameMeta({
        seq: seq++,
        timestampMs: frame.timestampMs,
        x: 0,
        y: 0,
        width: frame.width,
        height: frame.height,
        pixelFormat: PixelFormat.BGRA8,
        encoding,
        rawLength,
      });
      const payload = Buffer.concat([
        Buffer.from(metaBytes.buffer, metaBytes.byteOffset, metaBytes.byteLength),
        Buffer.from(body.buffer, body.byteOffset, body.length),
      ]);
      const flushed = active.conn.write(encodeMessage(MessageKind.Frame, payload));
      active.paused = !flushed;
      lastPixels = frame.data;
      this.counters.framesSent++;
      this.counters.bytesSent += payload.length;
    };

    const scheduleCapture = (): void => {
      if (host.closed) return;
      const interval = Math.max(1, Math.round(1000 / options.fps));
      host.captureTimer = setTimeout(() => {
        void captureTick()
          .catch((err: unknown) => {
            logger.error('capture loop failed', { error: String(err) });
          })
          .finally(scheduleCapture);
      }, interval);
    };

    const closeSession = (active: ActiveSession, reason: string): void => {
      if (active.closed) return;
      active.closed = true;
      if (active.handshakeTimer) clearTimeout(active.handshakeTimer);
      if (active.keepaliveTimer) clearInterval(active.keepaliveTimer);
      if (host.active === active) {
        host.active = null;
        this.deps.injector.releaseAll();
        lastPixels = null;
      }
      active.conn.close();
      this.deps.logger.info('session closed', { reason });
    };

    const failSession = (active: ActiveSession, code: string, message: string): void => {
      if (active.closed) return;
      try {
        active.conn.write(encodeMessage(MessageKind.Error, encodeError({ code, message })));
      } catch {
        // connection is likely already gone
      }
      closeSession(active, message);
    };

    const handleMessage = (active: ActiveSession, message: DecodedMessage): void => {
      active.lastSeenAt = Date.now();
      if (!active.authenticated) {
        if (message.kind !== MessageKind.Hello) {
          failSession(active, 'protocol', 'expected handshake message');
          return;
        }
        handleHello(active, message);
        return;
      }
      switch (message.kind) {
        case MessageKind.Input: {
          try {
            const event = decodeInputEvent(message.payload);
            this.deps.injector.inject(event);
            this.counters.inputEvents++;
          } catch (err) {
            this.deps.logger.warn('invalid input event', { error: String(err) });
          }
          break;
        }
        case MessageKind.Ping: {
          const ping = decodePing(decodeControlPayload(message));
          active.conn.write(encodeJSONMessage(MessageKind.Pong, { t: ping.t }));
          break;
        }
        case MessageKind.Pong:
          break;
        case MessageKind.Bye:
          closeSession(active, decodeBye(decodeControlPayload(message)).reason);
          break;
        case MessageKind.Hello:
          failSession(active, 'protocol', 'duplicate handshake');
          break;
        default:
          this.deps.logger.warn('unexpected message from viewer', { kind: message.kind });
      }
    };

    const handleHello = (active: ActiveSession, message: DecodedMessage): void => {
      const hello = decodeHello(decodeControlPayload(message));
      if (hello.protocolVersion !== PROTOCOL_VERSION) {
        active.conn.write(
          encodeMessage(
            MessageKind.Welcome,
            encodeWelcome({ ok: false, reason: `protocol version ${PROTOCOL_VERSION} required` }),
          ),
        );
        closeSession(active, 'protocol version mismatch');
        return;
      }
      if (!tokensEqual(hello.token, options.token)) {
        this.deps.logger.warn('rejected unauthorized viewer', { clientName: hello.clientName });
        active.conn.write(
          encodeMessage(MessageKind.Welcome, encodeWelcome({ ok: false, reason: 'unauthorized' })),
        );
        closeSession(active, 'unauthorized viewer');
        return;
      }
      if (active.handshakeTimer) {
        clearTimeout(active.handshakeTimer);
        active.handshakeTimer = null;
      }
      active.authenticated = true;
      this.counters.sessionsServed++;
      const bounds = this.deps.capturer.bounds();
      lastPixels = null;
      active.conn.write(
        encodeMessage(
          MessageKind.Welcome,
          encodeWelcome({
            ok: true,
            serverVersion: SERVER_VERSION,
            screen: { width: bounds.width, height: bounds.height },
            fps: options.fps,
          }),
        ),
      );
      this.deps.logger.info('viewer connected', { clientName: hello.clientName });
    };

    const acceptConnection = (conn: Conn): void => {
      this.counters.connectionsAccepted++;
      if (host.active && !host.active.closed) {
        conn.write(
          encodeMessage(MessageKind.Error, encodeError({ code: 'busy', message: 'another viewer is already connected' })),
        );
        conn.close();
        this.deps.logger.warn('rejected viewer: session busy', { remote: conn.remoteAddress });
        return;
      }
      const active: ActiveSession = {
        conn,
        decoder: new MessageDecoder(),
        authenticated: false,
        closed: false,
        lastSeenAt: Date.now(),
        handshakeTimer: null,
        keepaliveTimer: null,
        paused: false,
      };
      host.active = active;
      this.deps.logger.info('viewer connecting', { remote: conn.remoteAddress });

      active.handshakeTimer = setTimeout(() => {
        failSession(active, 'timeout', 'handshake timed out');
      }, options.handshakeTimeoutMs);

      active.keepaliveTimer = setInterval(() => {
        if (active.closed) return;
        if (Date.now() - active.lastSeenAt > options.pingTimeoutMs) {
          closeSession(active, 'ping timeout');
          return;
        }
        conn.write(encodeJSONMessage(MessageKind.Ping, { t: Date.now() }));
      }, options.pingIntervalMs);

      conn.onDrain(() => {
        active.paused = false;
      });
      conn.onData((chunk) => {
        if (active.closed) return;
        let messages: DecodedMessage[];
        try {
          messages = active.decoder.feed(chunk);
        } catch (err) {
          const gw = toGhostwireError(err, 'protocol');
          failSession(active, gw.code, gw.message);
          return;
        }
        for (const message of messages) {
          if (active.closed) break;
          try {
            handleMessage(active, message);
          } catch (err) {
            const gw = toGhostwireError(err, 'protocol');
            failSession(active, gw.code, gw.message);
            break;
          }
        }
      });
      conn.onError((err) => {
        this.deps.logger.debug('connection error', { error: err.message });
        closeSession(active, err.message);
      });
      conn.onClose(() => {
        closeSession(active, 'peer closed');
      });
    };

    listener.onConnection(acceptConnection);
    listener.onError((err) => {
      this.deps.logger.error('listener error', { error: err.message });
    });
    scheduleCapture();

    try {
      await new Promise<void>((resolve) => {
        if (signal.aborted) resolve();
        else signal.addEventListener('abort', () => resolve(), { once: true });
      });
    } finally {
      host.closed = true;
      if (host.captureTimer) clearTimeout(host.captureTimer);
      if (host.active && !host.active.closed) {
        try {
          host.active.conn.write(encodeJSONMessage(MessageKind.Bye, { reason: 'server shutting down' }));
        } catch {
          // ignore: peer may be gone
        }
      }
      if (host.active) closeSession(host.active, 'server shutting down');
      try {
        await listener.close();
      } catch (err) {
        this.deps.logger.debug('listener close failed', { error: String(err) });
      }
      await this.deps.capturer.close();
      this.deps.injector.close();
      this.host = null;
      this.deps.logger.info('server stopped');
    }
  }
}
