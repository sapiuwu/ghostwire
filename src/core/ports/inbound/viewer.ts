import type { InputEvent } from '../../domain/input/events.js';
import type { ScreenFrame, ScreenSize } from '../../domain/screen/frame.js';
import type { ConnectOptions, SessionInfo, SessionStats } from '../../domain/session/config.js';

export type SessionEvent =
  | { kind: 'frame'; frame: ScreenFrame }
  | { kind: 'screen'; screen: ScreenSize };

export interface SessionPort {
  info(): SessionInfo;
  events(): AsyncIterable<SessionEvent>;
  sendInput(event: InputEvent): void;
  stats(): SessionStats;
  close(reason?: string): Promise<void>;
}

export interface ViewerPort {
  connect(options: ConnectOptions): Promise<SessionPort>;
}
