import type { TlsDialOptions, TlsListenOptions } from '../src/core/domain/session/config.js';
import type { ScreenFrame, ScreenSize } from '../src/core/domain/screen/frame.js';
import type { InputEvent } from '../src/core/domain/input/events.js';
import type { CaptureOptions, ScreenCapturerPort } from '../src/core/ports/outbound/capture.js';
import type { InputInjectorPort } from '../src/core/ports/outbound/inject.js';
import type { Logger } from '../src/core/ports/outbound/logger.js';
import type { Conn, Listener, TransportPort } from '../src/core/ports/outbound/transport.js';
import { GhostwireError } from '../src/core/domain/errors.js';

export class MemoryConn implements Conn {
  readonly id: string;
  readonly remoteAddress = 'memory:0';
  peer: MemoryConn | null = null;
  closed = false;
  written: Buffer[] = [];

  private dataCb: ((chunk: Buffer) => void) | null = null;
  private errorCb: ((err: Error) => void) | null = null;
  private closeCb: (() => void) | null = null;
  private drainCb: (() => void) | null = null;
  private seq = 0;

  constructor(id: string) {
    this.id = id;
  }

  write(chunk: Buffer): boolean {
    if (this.closed) return false;
    this.written.push(chunk);
    const peer = this.peer;
    if (peer) {
      const copy = Buffer.from(chunk);
      queueMicrotask(() => peer.deliver(copy));
    }
    queueMicrotask(() => this.drainCb?.());
    return true;
  }

  private deliver(chunk: Buffer): void {
    if (this.closed) return;
    this.dataCb?.(chunk);
  }

  onData(cb: (chunk: Buffer) => void): void {
    this.dataCb = cb;
  }

  onError(cb: (err: Error) => void): void {
    this.errorCb = cb;
  }

  onClose(cb: () => void): void {
    this.closeCb = cb;
  }

  onDrain(cb: () => void): void {
    this.drainCb = cb;
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.closeCb?.();
    const peer = this.peer;
    if (peer && !peer.closed) queueMicrotask(() => peer.close());
  }

  fail(err: Error): void {
    this.errorCb?.(err);
  }
}

export class MemoryListener implements Listener {
  readonly address: string;
  closed = false;
  private connectionCb: ((conn: Conn) => void) | null = null;
  private errorCb: ((err: Error) => void) | null = null;

  constructor(address: string) {
    this.address = address;
  }

  onConnection(cb: (conn: Conn) => void): void {
    this.connectionCb = cb;
  }

  onError(cb: (err: Error) => void): void {
    this.errorCb = cb;
  }

  accept(conn: Conn): void {
    this.connectionCb?.(conn);
  }

  fail(err: Error): void {
    this.errorCb?.(err);
  }

  close(): Promise<void> {
    this.closed = true;
    return Promise.resolve();
  }
}

export class MemoryTransport implements TransportPort {
  readonly listeners: MemoryListener[] = [];
  lastClient: MemoryConn | null = null;
  lastServer: MemoryConn | null = null;
  private counter = 0;

  async listen(address: string, _tls?: TlsListenOptions): Promise<Listener> {
    const listener = new MemoryListener(address);
    this.listeners.push(listener);
    return listener;
  }

  async dial(_address: string, _tls?: TlsDialOptions): Promise<Conn> {
    const listener = this.listeners.find((candidate) => !candidate.closed);
    if (!listener) throw new GhostwireError('transport', 'no listener');
    const client = new MemoryConn(`client-${++this.counter}`);
    const server = new MemoryConn(`server-${this.counter}`);
    client.peer = server;
    server.peer = client;
    this.lastClient = client;
    this.lastServer = server;
    queueMicrotask(() => listener.accept(server));
    return client;
  }
}

export class FakeCapturer implements ScreenCapturerPort {
  width = 8;
  height = 4;
  started = false;
  closed = false;
  private counter = 0;

  async start(_options: CaptureOptions): Promise<void> {
    this.started = true;
  }

  bounds(): ScreenSize {
    return { width: this.width, height: this.height };
  }

  grab(): ScreenFrame {
    const data = new Uint8Array(this.width * this.height * 4);
    const stamp = ++this.counter;
    for (let i = 0; i < data.length; i += 4) {
      data[i] = (i + stamp) & 0xff;
      data[i + 1] = (i * 2 + stamp) & 0xff;
      data[i + 2] = (i * 3 + stamp) & 0xff;
      data[i + 3] = 255;
    }
    return {
      width: this.width,
      height: this.height,
      format: 'bgra8',
      timestampMs: Date.now(),
      data,
    };
  }

  async close(): Promise<void> {
    this.closed = true;
  }
}

export class FakeInjector implements InputInjectorPort {
  events: InputEvent[] = [];
  released = false;
  closed = false;

  inject(event: InputEvent): void {
    this.events.push(event);
  }

  releaseAll(): void {
    this.released = true;
  }

  close(): void {
    this.closed = true;
  }
}

export class TestLogger implements Logger {
  readonly level = 'debug' as const;
  readonly lines: string[] = [];

  debug(message: string, meta?: Record<string, unknown>): void {
    this.push('debug', message, meta);
  }

  info(message: string, meta?: Record<string, unknown>): void {
    this.push('info', message, meta);
  }

  warn(message: string, meta?: Record<string, unknown>): void {
    this.push('warn', message, meta);
  }

  error(message: string, meta?: Record<string, unknown>): void {
    this.push('error', message, meta);
  }

  private push(level: string, message: string, meta?: Record<string, unknown>): void {
    this.lines.push(meta ? `${level} ${message} ${JSON.stringify(meta)}` : `${level} ${message}`);
  }
}

export async function waitFor(predicate: () => boolean, timeoutMs = 3000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() > deadline) throw new Error('waitFor timed out');
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
}

export async function delay(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}
