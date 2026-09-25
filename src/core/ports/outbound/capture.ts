import type { ScreenFrame, ScreenSize } from '../../domain/screen/frame.js';

export interface CaptureOptions {
  maxWidth: number;
}

export interface ScreenCapturerPort {
  start(options: CaptureOptions): Promise<void>;
  bounds(): ScreenSize;
  grab(): ScreenFrame;
  close(): Promise<void>;
}
