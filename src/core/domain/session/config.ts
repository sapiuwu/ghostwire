export interface TlsListenOptions {
  certPath: string;
  keyPath: string;
}

export interface TlsDialOptions {
  caCertPath?: string;
  insecure?: boolean;
}

export interface ServeOptions {
  address: string;
  token: string;
  fps: number;
  maxWidth: number;
  compress: 'deflate' | 'none';
  handshakeTimeoutMs: number;
  pingIntervalMs: number;
  pingTimeoutMs: number;
  tls?: TlsListenOptions;
}

export interface ConnectOptions {
  address: string;
  token: string;
  clientName: string;
  handshakeTimeoutMs: number;
  pingIntervalMs: number;
  pingTimeoutMs: number;
  tls?: TlsDialOptions;
}

export interface SessionInfo {
  screen: { width: number; height: number };
  fps: number;
  serverVersion: string;
}

export interface SessionStats {
  framesReceived: number;
  framesDropped: number;
  bytesReceived: number;
  rttMs: number;
  fps: number;
}
