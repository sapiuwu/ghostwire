import { GhostwireError } from '../../../core/domain/errors.js';
import { Key, Modifier, type InputEvent } from '../../../core/domain/input/events.js';
import type { ConnectOptions, SessionInfo } from '../../../core/domain/session/config.js';
import type { ScreenFrame, ScreenSize } from '../../../core/domain/screen/frame.js';
import type { SessionEvent, SessionPort, ViewerPort } from '../../../core/ports/inbound/viewer.js';
import type { Logger } from '../../../core/ports/outbound/logger.js';
import { AnsiRenderer } from './ansi-renderer.js';
import { mapCellToScreen, parseTerminalInput } from './input-parser.js';

export interface TerminalViewerDeps {
  viewer: ViewerPort;
  logger: Logger;
  stdin: typeof process.stdin;
  stdout: typeof process.stdout;
}

const STATUS_INTERVAL_MS = 500;
const ESC_FLUSH_MS = 50;

export class TerminalViewer {
  constructor(private readonly deps: TerminalViewerDeps) {}

  async run(options: ConnectOptions, signal: AbortSignal): Promise<void> {
    const { stdin, stdout, viewer, logger } = this.deps;
    if (!stdout.isTTY || !stdin.isTTY) {
      throw new GhostwireError('unsupported', 'connect: terminal viewer requires an interactive TTY');
    }

    const session = await viewer.connect(options);
    let screen: ScreenSize = session.info().screen;
    let quitRequested = false;
    let lastStatusAt = 0;

    const renderer = new AnsiRenderer(
      { write: (chunk) => void stdout.write(chunk) },
      stdout.columns ?? 80,
      stdout.rows ?? 24,
      true,
    );

    const mapCell = (cellX: number, cellY: number): { x: number; y: number } =>
      mapCellToScreen(cellX, cellY, screen, renderer.cols, renderer.viewportRows);

    const applyInput = (events: InputEvent[]): void => {
      for (const event of events) {
        if (quitRequested) return;
        if (event.kind === 'key' && event.key === Key.Q && (event.modifiers & Modifier.Ctrl) !== 0) {
          if (event.down) {
            quitRequested = true;
            void session.close('viewer quit').catch(() => undefined);
          }
          continue;
        }
        session.sendInput(event);
      }
    };

    let pending: Uint8Array = new Uint8Array(0);
    let flushTimer: NodeJS.Timeout | null = null;

    const onData = (chunk: Buffer | string): void => {
      const bytes = typeof chunk === 'string' ? Buffer.from(chunk, 'latin1') : chunk;
      const merged =
        pending.length === 0
          ? new Uint8Array(bytes.buffer, bytes.byteOffset, bytes.byteLength)
          : concatBytes(pending, bytes);
      const parsed = parseTerminalInput(merged, { mapCell }, false);
      applyInput(parsed.events);
      pending = merged.subarray(parsed.consumed);
      if (flushTimer !== null) clearTimeout(flushTimer);
      flushTimer = null;
      if (pending.length > 0 && !quitRequested) {
        flushTimer = setTimeout(() => {
          flushTimer = null;
          if (pending.length === 0 || quitRequested) return;
          const flushed = parseTerminalInput(pending, { mapCell }, true);
          applyInput(flushed.events);
          pending =
            flushed.consumed > 0 ? pending.subarray(flushed.consumed) : new Uint8Array(0);
        }, ESC_FLUSH_MS);
      }
    };

    const onResize = (): void => {
      renderer.resize(stdout.columns ?? 80, stdout.rows ?? 24);
    };

    const onAbort = (): void => {
      void session.close('aborted').catch(() => undefined);
    };

    const wasRaw = stdin.isRaw ?? false;
    renderer.begin();
    stdin.setRawMode(true);
    stdin.resume();
    stdin.on('data', onData);
    stdout.on('resize', onResize);
    signal.addEventListener('abort', onAbort, { once: true });

    let exitError: unknown = null;
    try {
      for await (const event of session.events()) {
        handleSessionEvent(event, {
          onFrame: (frame) => {
            renderer.render(frame);
            const now = Date.now();
            if (now - lastStatusAt >= STATUS_INTERVAL_MS) {
              lastStatusAt = now;
              renderer.status(statusLine(session));
            }
          },
          onScreen: (size) => {
            screen = size;
            onResize();
          },
        });
      }
    } catch (err) {
      exitError = err;
      throw err;
    } finally {
      if (flushTimer !== null) clearTimeout(flushTimer);
      stdin.off('data', onData);
      stdout.off('resize', onResize);
      signal.removeEventListener('abort', onAbort);
      stdin.pause();
      stdin.setRawMode(wasRaw);
      renderer.end();
      logger.debug('terminal viewer cleaned up', {
        quit: quitRequested,
        error: exitError === null ? undefined : String(exitError),
      });
    }
  }
}

function handleSessionEvent(
  event: SessionEvent,
  handlers: {
    onFrame(frame: ScreenFrame): void;
    onScreen(screen: ScreenSize): void;
  },
): void {
  if (event.kind === 'frame') handlers.onFrame(event.frame);
  else handlers.onScreen(event.screen);
}

function statusLine(session: SessionPort): string {
  const info: SessionInfo = session.info();
  const stats = session.stats();
  const size = `${info.screen.width}x${info.screen.height}`;
  const fps = Number.isFinite(stats.fps) ? Math.max(0, Math.round(stats.fps)) : 0;
  const rtt = stats.rttMs > 0 ? `${Math.round(stats.rttMs)}ms` : '-';
  const dropped = stats.framesDropped > 0 ? ` drop ${stats.framesDropped}` : '';
  return ` ghostwire | ${size} | ${fps} fps | rtt ${rtt}${dropped} | ctrl+q quit`;
}

function concatBytes(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length + b.length);
  out.set(a, 0);
  out.set(b, a.length);
  return out;
}
