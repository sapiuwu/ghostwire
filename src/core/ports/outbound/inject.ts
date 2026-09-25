import type { InputEvent } from '../../domain/input/events.js';

export interface InputInjectorPort {
  inject(event: InputEvent): void;
  releaseAll(): void;
  close(): void;
}
