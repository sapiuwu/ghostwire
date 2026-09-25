import { readFileSync } from 'node:fs';
import net from 'node:net';
import tls from 'node:tls';
import { GhostwireError } from '../../../core/domain/errors.js';
import type { TlsDialOptions, TlsListenOptions } from '../../../core/domain/session/config.js';
import type { Conn, Listener, TransportPort } from '../../../core/ports/outbound/transport.js';

const CONNECT_TIMEOUT_MS = 10_000;
const CLOSE_LINGER_MS = 2_000;

export function parseAddress(address: string): { host: string; port: number } {
  const index = address.lastIndexOf(':');
  if (index < 0) {
    throw new GhostwireError('transport', `invalid address "${address}" (expected host:port)`);
  }
  let host = address.slice(0, index);
  if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1);
  const port = Number(address.slice(index + 1));
  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new GhostwireError('transport', `invalid port in address "${address}"`);
  }
  return { host, port };
}

let connSeq = 0;

class SocketConn implements Conn {
  readonly id: string;
  readonly remoteAddress: string;
  private readonly socket: net.Socket | tls.TLSSocket;
  private closing = false;

  constructor(socket: net.Socket | tls.TLSSocket) {
    this.socket = socket;
    this.id = `conn-${++connSeq}`;
    const host = socket.remoteAddress ?? 'unknown';
    const port = socket.remotePort ?? 0;
    this.remoteAddress = `${host}:${port}`;
    socket.setNoDelay(true);
    socket.on('error', () => {
      // consumers register their own handler; this prevents unhandled 'error' before they do
    });
  }

  write(chunk: Buffer): boolean {
    if (this.closing || this.socket.destroyed) return false;
    return this.socket.write(chunk);
  }

  onData(cb: (chunk: Buffer) => void): void {
    this.socket.on('data', cb);
  }

  onError(cb: (err: Error) => void): void {
    this.socket.on('error', cb);
  }

  onClose(cb: () => void): void {
    this.socket.on('close', cb);
  }

  onDrain(cb: () => void): void {
    this.socket.on('drain', cb);
  }

  close(): void {
    if (this.closing) return;
    this.closing = true;
    if (this.socket.destroyed) return;
    this.socket.end();
    const timer = setTimeout(() => {
      this.socket.destroy();
    }, CLOSE_LINGER_MS);
    timer.unref();
  }
}

class SocketListener implements Listener {
  readonly address: string;
  private readonly server: net.Server;
  private readonly sockets = new Set<net.Socket>();
  private closed = false;

  constructor(server: net.Server, address: string) {
    this.server = server;
    this.address = address;
    server.on('connection', (socket) => {
      this.sockets.add(socket);
      socket.on('close', () => this.sockets.delete(socket));
    });
  }

  onConnection(cb: (conn: Conn) => void): void {
    this.server.on('connection', (socket) => {
      cb(new SocketConn(socket));
    });
  }

  onError(cb: (err: Error) => void): void {
    this.server.on('error', cb);
  }

  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;
    for (const socket of this.sockets) socket.destroy();
    this.sockets.clear();
    await new Promise<void>((resolve) => {
      this.server.close(() => resolve());
    });
  }
}

function formatAddress(host: string, port: number): string {
  const display = host === '' ? '0.0.0.0' : host;
  return display.includes(':') ? `[${display}]:${port}` : `${display}:${port}`;
}

export class TcpTransport implements TransportPort {
  async listen(address: string, tlsOptions?: TlsListenOptions): Promise<Listener> {
    const { host, port } = parseAddress(address);
    const server = tlsOptions ? this.createTlsServer(tlsOptions) : net.createServer();

    await new Promise<void>((resolve, reject) => {
      const onError = (err: Error): void => {
        server.removeListener('listening', onListening);
        reject(new GhostwireError('transport', err.message, { cause: err }));
      };
      const onListening = (): void => {
        server.removeListener('error', onError);
        resolve();
      };
      server.once('error', onError);
      server.once('listening', onListening);
      server.listen(port, host === '' ? undefined : host);
    });

    const bound = server.address();
    const boundPort = typeof bound === 'object' && bound !== null ? bound.port : port;
    const boundHost = typeof bound === 'object' && bound !== null && bound.address ? bound.address : host;
    const listener = new SocketListener(server, formatAddress(boundHost, boundPort));
    return listener;
  }

  async dial(address: string, tlsOptions?: TlsDialOptions): Promise<Conn> {
    const { host, port } = parseAddress(address);
    const connectHost = host === '' ? '127.0.0.1' : host;
    const socket = await new Promise<net.Socket | tls.TLSSocket>((resolve, reject) => {
      const opts: net.NetConnectOpts = { host: connectHost, port };
      const stream = tlsOptions
        ? tls.connect(this.tlsConnectOptions(tlsOptions, connectHost, port))
        : net.connect(opts);
      const timer = setTimeout(() => {
        stream.destroy();
        reject(new GhostwireError('timeout', `connection to ${address} timed out`));
      }, CONNECT_TIMEOUT_MS);
      const onError = (err: Error): void => {
        clearTimeout(timer);
        stream.destroy();
        reject(new GhostwireError('transport', `failed to connect to ${address}: ${err.message}`, { cause: err }));
      };
      const onConnect = (): void => {
        clearTimeout(timer);
        stream.removeListener('error', onError);
        resolve(stream);
      };
      stream.once('error', onError);
      stream.once(tlsOptions ? 'secureConnect' : 'connect', onConnect);
    });
    return new SocketConn(socket);
  }

  private createTlsServer(options: TlsListenOptions): tls.Server {
    let cert: Buffer;
    let key: Buffer;
    try {
      cert = readFileSync(options.certPath);
      key = readFileSync(options.keyPath);
    } catch (err) {
      throw new GhostwireError('transport', `failed to read TLS credentials: ${String(err)}`, { cause: err });
    }
    return tls.createServer({ cert, key });
  }

  private tlsConnectOptions(options: TlsDialOptions, host: string, port: number): tls.ConnectionOptions {
    const opts: tls.ConnectionOptions = {
      host,
      port,
      rejectUnauthorized: options.insecure !== true,
    };
    if (!net.isIP(host)) opts.servername = host;
    if (options.caCertPath) {
      try {
        opts.ca = readFileSync(options.caCertPath);
      } catch (err) {
        throw new GhostwireError('transport', `failed to read CA certificate: ${String(err)}`, { cause: err });
      }
    }
    return opts;
  }
}
