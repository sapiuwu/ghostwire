import type { TlsDialOptions, TlsListenOptions } from '../../domain/session/config.js';

export interface Conn {
  readonly id: string;
  readonly remoteAddress: string;
  write(chunk: Buffer): boolean;
  onData(cb: (chunk: Buffer) => void): void;
  onError(cb: (err: Error) => void): void;
  onClose(cb: () => void): void;
  onDrain(cb: () => void): void;
  close(): void;
}

export interface Listener {
  readonly address: string;
  onConnection(cb: (conn: Conn) => void): void;
  onError(cb: (err: Error) => void): void;
  close(): Promise<void>;
}

export interface TransportPort {
  listen(address: string, tls?: TlsListenOptions): Promise<Listener>;
  dial(address: string, tls?: TlsDialOptions): Promise<Conn>;
}
