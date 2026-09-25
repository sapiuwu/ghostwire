import type { ServeOptions } from '../../domain/session/config.js';

export interface ServerStats {
  connectionsAccepted: number;
  sessionsServed: number;
  framesSent: number;
  framesSkipped: number;
  inputEvents: number;
  bytesSent: number;
}

export interface ServerPort {
  serve(options: ServeOptions, signal: AbortSignal): Promise<void>;
  stats(): ServerStats;
}
