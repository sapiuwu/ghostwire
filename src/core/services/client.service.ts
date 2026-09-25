import { GhostwireError, toGhostwireError } from '../domain/errors.js';
import { encodeInputEvent } from '../domain/input/events.js';
import {
  FrameEncoding,
  MessageKind,
  PROTOCOL_VERSION,
} from '../domain/protocol/constants.js';
import { MessageDecoder, decodeControlPayload, encodeJSONMessage, encodeMessage } from '../domain/protocol/codec.js';
import { decodeFrameMeta } from '../domain/protocol/frame.js';
import {
  decodeBye,
  decodeError,
  decodePing,
  decodeScreenInfo,
  decodeWelcome,
} from '../domain/protocol/messages.js';
import type { DecodedMessage } from '../domain/protocol/codec.js';
import type { ScreenFrame } from '../domain/screen/frame.js';
import type { ConnectOptions, SessionInfo, SessionStats } from '../domain/session/config.js';
import type { SessionEvent, SessionPort, ViewerPort } from '../ports/inbound/viewer.js';
import { AsyncQueue } from '../util/async-queue.js';
import type { CompressorPort } from '../ports/outbound/compress.js';
import type { Logger } from '../ports/outbound/logger.js';
import type { TransportPort } from '../ports/outbound/transport.js';

export interface ClientServiceDeps {
  transport: TransportPort;
  compressor: CompressorPort;
  logger: Logger;
}

const EVENT_QUEUE_LIMIT = 4;

export class ClientService implements ViewerPort {
  private readonly deps: ClientServiceDeps;

  constructor(deps: ClientServiceDeps) {
    this.deps = deps;
  }

  async connect(options: ConnectOptions): Promise<SessionPort> {
    const { transport, compressor, logger } = this.deps;
    const conn = await transport.dial(options.address, options.tls);
    const decoder = new MessageDecoder();
    const inbox: DecodedMessage[] = [];
    const queue = new AsyncQueue<SessionEvent>();
    let notify: (() => void) | null = null;
    let closed = false;
    let closeError: GhostwireError | null = null;
    let keepalive: NodeJS.Timeout | null = null;

    const wake = (): void => {
      if (notify) {
        const fn = notify;
        notify = null;
        fn();
      }
    };

    const finish = (err: GhostwireError | null): void => {
      if (closed) return;
      closed = true;
      closeError = err;
      if (keepalive) clearInterval(keepalive);
      keepalive = null;
      queue.end(err ?? undefined);
      conn.close();
      wake();
      if (err) logger.debug('session ended with error', { error: err.message });
    };

    conn.onData((chunk) => {
      if (closed) return;
      let messages: DecodedMessage[];
      try {
        messages = decoder.feed(chunk);
      } catch (err) {
        finish(toGhostwireError(err, 'protocol'));
        return;
      }
      for (const message of messages) inbox.push(message);
      wake();
    });
    conn.onError((err) => {
      finish(new GhostwireError('transport', err.message, { cause: err }));
    });
    conn.onClose(() => {
      finish(closeError ?? new GhostwireError('closed', 'connection closed by peer'));
    });
    conn.onDrain(() => undefined);

    const waitForInbox = async (deadlineAt: number): Promise<void> => {
      if (inbox.length > 0) return;
      const remaining = deadlineAt - Date.now();
      if (remaining <= 0) {
        throw new GhostwireError('timeout', 'timed out waiting for server response');
      }
      await new Promise<void>((resolve) => {
        const timer = setTimeout(() => {
          notify = null;
          resolve();
        }, remaining);
        notify = () => {
          clearTimeout(timer);
          resolve();
        };
      });
      if (inbox.length === 0) {
        if (closed) {
          throw closeError ?? new GhostwireError('closed', 'connection closed during handshake');
        }
        throw new GhostwireError('timeout', 'timed out waiting for server response');
      }
    };

    const info: SessionInfo = { screen: { width: 0, height: 0 }, fps: 0, serverVersion: '' };
    const stats: SessionStats = {
      framesReceived: 0,
      framesDropped: 0,
      bytesReceived: 0,
      rttMs: 0,
      fps: 0,
    };
    const frameTimes: number[] = [];

    conn.write(
      encodeJSONMessage(MessageKind.Hello, {
        protocolVersion: PROTOCOL_VERSION,
        clientName: options.clientName,
        token: options.token,
      }),
    );

    const handshakeDeadline = Date.now() + options.handshakeTimeoutMs;
    while (true) {
      await waitForInbox(handshakeDeadline);
      const message = inbox[0]!;
      if (message.kind === MessageKind.Welcome) {
        inbox.shift();
        const welcome = decodeWelcome(decodeControlPayload(message));
        if (!welcome.ok) {
          const reason = welcome.reason ?? 'rejected by server';
          conn.write(encodeJSONMessage(MessageKind.Bye, { reason }));
          finish(new GhostwireError(reason === 'unauthorized' ? 'auth' : 'protocol', reason));
          throw closeError!;
        }
        info.screen = welcome.screen ?? { width: 0, height: 0 };
        info.fps = welcome.fps ?? 0;
        info.serverVersion = welcome.serverVersion ?? 'unknown';
        break;
      }
      if (message.kind === MessageKind.Error) {
        inbox.shift();
        const payload = decodeError(decodeControlPayload(message));
        finish(new GhostwireError('protocol', `${payload.code}: ${payload.message}`));
        throw closeError!;
      }
      if (message.kind === MessageKind.Bye) {
        inbox.shift();
        const payload = decodeByeSafe(message);
        finish(new GhostwireError('closed', payload));
        throw closeError!;
      }
      if (message.kind === MessageKind.Hello || message.kind === MessageKind.Frame || message.kind === MessageKind.Input) {
        finish(new GhostwireError('protocol', 'unexpected message during handshake'));
        throw closeError!;
      }
      inbox.shift();
    }

    let lastSeenAt = Date.now();

    const enqueue = (event: SessionEvent): void => {
      if (event.kind !== 'frame') {
        queue.push(event);
        return;
      }
      if (queue.size >= EVENT_QUEUE_LIMIT) {
        const dropped = queue.removeFirst((existing) => existing.kind === 'frame');
        if (!dropped) {
          stats.framesDropped++;
          return;
        }
        stats.framesDropped++;
      }
      queue.push(event);
    };

    const processMessage = async (message: DecodedMessage): Promise<void> => {
      lastSeenAt = Date.now();
      switch (message.kind) {
        case MessageKind.Frame: {
          const meta = decodeFrameMeta(message.payload);
          const body = message.payload.subarray(32);
          let pixels: Uint8Array;
          if (meta.encoding === FrameEncoding.Deflate) {
            if (compressor.name !== 'deflate') {
              throw new GhostwireError('protocol', 'frame is deflated but no decompressor configured');
            }
            pixels = await compressor.decompress(body, meta.rawLength);
          } else {
            pixels = body;
          }
          if (pixels.length !== meta.rawLength) {
            throw new GhostwireError('protocol', 'decompressed frame has unexpected size');
          }
          const frame: ScreenFrame = {
            width: meta.width,
            height: meta.height,
            format: 'bgra8',
            timestampMs: meta.timestampMs,
            data: pixels,
          };
          stats.framesReceived++;
          stats.bytesReceived += message.payload.length;
          frameTimes.push(Date.now());
          enqueue({ kind: 'frame', frame });
          break;
        }
        case MessageKind.ScreenInfo: {
          const screen = decodeScreenInfo(decodeControlPayload(message));
          info.screen = screen;
          enqueue({ kind: 'screen', screen });
          break;
        }
        case MessageKind.Ping: {
          const ping = decodePing(decodeControlPayload(message));
          conn.write(encodeJSONMessage(MessageKind.Pong, { t: ping.t }));
          break;
        }
        case MessageKind.Pong: {
          const pong = decodePing(decodeControlPayload(message));
          const rtt = Date.now() - pong.t;
          if (rtt >= 0 && rtt < 60_000) stats.rttMs = rtt;
          break;
        }
        case MessageKind.Error: {
          const payload = decodeError(decodeControlPayload(message));
          throw new GhostwireError('protocol', `${payload.code}: ${payload.message}`);
        }
        case MessageKind.Bye: {
          const reason = decodeByeSafe(message);
          throw new GhostwireError('closed', `server closed session: ${reason}`);
        }
        default:
          logger.warn('unexpected message from server', { kind: message.kind });
      }
    };

    const pump = async (): Promise<void> => {
      while (!closed) {
        if (inbox.length === 0) {
          await new Promise<void>((resolve) => {
            notify = () => {
              notify = null;
              resolve();
            };
          });
          continue;
        }
        const message = inbox.shift()!;
        await processMessage(message);
      }
    };

    keepalive = setInterval(() => {
      if (closed) return;
      if (Date.now() - lastSeenAt > options.pingTimeoutMs) {
        finish(new GhostwireError('timeout', 'server stopped responding'));
        return;
      }
      conn.write(encodeJSONMessage(MessageKind.Ping, { t: Date.now() }));
    }, options.pingIntervalMs);

    void pump()
      .catch((err: unknown) => {
        finish(toGhostwireError(err, 'protocol'));
      })
      .finally(() => {
        // pump exits when the session finishes
      });

    const session: SessionPort = {
      info: () => ({
        screen: { ...info.screen },
        fps: info.fps,
        serverVersion: info.serverVersion,
      }),
      events: () => queue,
      sendInput: (event) => {
        if (closed) return;
        conn.write(encodeMessage(MessageKind.Input, encodeInputEvent(event)));
      },
      stats: () => {
        const now = Date.now();
        while (frameTimes.length > 0 && now - frameTimes[0]! > 1000) frameTimes.shift();
        stats.fps = frameTimes.length;
        return { ...stats };
      },
      close: async (reason = 'viewer closed') => {
        if (closed) return;
        try {
          conn.write(encodeJSONMessage(MessageKind.Bye, { reason }));
        } catch {
          // ignore: peer may be gone
        }
        finish(null);
        await Promise.resolve();
      },
    };
    return session;
  }
}

function decodeByeSafe(message: DecodedMessage): string {
  try {
    return decodeBye(decodeControlPayload(message)).reason;
  } catch {
    return 'bye';
  }
}
