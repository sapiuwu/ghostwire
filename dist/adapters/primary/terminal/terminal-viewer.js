import { GhostwireError } from '../../../core/domain/errors.js';
import { Key, Modifier } from '../../../core/domain/input/events.js';
import { AnsiRenderer } from './ansi-renderer.js';
import { parseTerminalInput } from './input-parser.js';
const STATUS_INTERVAL_MS = 500;
const ESC_FLUSH_MS = 50;
export class TerminalViewer {
    deps;
    constructor(deps) {
        this.deps = deps;
    }
    async run(options, signal) {
        const { stdin, stdout, viewer, logger } = this.deps;
        if (!stdout.isTTY || !stdin.isTTY) {
            throw new GhostwireError('unsupported', 'connect: terminal viewer requires an interactive TTY');
        }
        const session = await viewer.connect(options);
        let screen = session.info().screen;
        let quitRequested = false;
        let lastStatusAt = 0;
        const renderer = new AnsiRenderer({ write: (chunk) => void stdout.write(chunk) }, stdout.columns ?? 80, stdout.rows ?? 24, true);
        const mapCell = (cellX, cellY) => {
            if (screen.width <= 0 || screen.height <= 0)
                return { x: 0, y: 0 };
            const col = Math.min(Math.max(Math.trunc(cellX), 1), renderer.cols);
            const row = Math.min(Math.max(Math.trunc(cellY), 1), renderer.viewportRows);
            const x = Math.floor(((col - 1) + 0.5) * (screen.width / renderer.cols));
            const y = Math.floor((2 * row - 0.5) * (screen.height / (2 * renderer.viewportRows)));
            return {
                x: Math.min(x, screen.width - 1),
                y: Math.min(y, screen.height - 1),
            };
        };
        const applyInput = (events) => {
            for (const event of events) {
                if (quitRequested)
                    return;
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
        let pending = new Uint8Array(0);
        let flushTimer = null;
        const onData = (chunk) => {
            const bytes = typeof chunk === 'string' ? Buffer.from(chunk, 'latin1') : chunk;
            const merged = pending.length === 0
                ? new Uint8Array(bytes.buffer, bytes.byteOffset, bytes.byteLength)
                : concatBytes(pending, bytes);
            const parsed = parseTerminalInput(merged, { mapCell }, false);
            applyInput(parsed.events);
            pending = merged.subarray(parsed.consumed);
            if (flushTimer !== null)
                clearTimeout(flushTimer);
            flushTimer = null;
            if (pending.length > 0 && !quitRequested) {
                flushTimer = setTimeout(() => {
                    flushTimer = null;
                    if (pending.length === 0 || quitRequested)
                        return;
                    const flushed = parseTerminalInput(pending, { mapCell }, true);
                    applyInput(flushed.events);
                    pending =
                        flushed.consumed > 0 ? pending.subarray(flushed.consumed) : new Uint8Array(0);
                }, ESC_FLUSH_MS);
            }
        };
        const onResize = () => {
            renderer.resize(stdout.columns ?? 80, stdout.rows ?? 24);
        };
        const onAbort = () => {
            void session.close('aborted').catch(() => undefined);
        };
        const wasRaw = stdin.isRaw ?? false;
        renderer.begin();
        stdin.setRawMode(true);
        stdin.resume();
        stdin.on('data', onData);
        stdout.on('resize', onResize);
        signal.addEventListener('abort', onAbort, { once: true });
        let exitError = null;
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
        }
        catch (err) {
            exitError = err;
            throw err;
        }
        finally {
            if (flushTimer !== null)
                clearTimeout(flushTimer);
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
function handleSessionEvent(event, handlers) {
    if (event.kind === 'frame')
        handlers.onFrame(event.frame);
    else
        handlers.onScreen(event.screen);
}
function statusLine(session) {
    const info = session.info();
    const stats = session.stats();
    const size = `${info.screen.width}x${info.screen.height}`;
    const fps = Number.isFinite(stats.fps) ? Math.max(0, Math.round(stats.fps)) : 0;
    const rtt = stats.rttMs > 0 ? `${Math.round(stats.rttMs)}ms` : '-';
    const dropped = stats.framesDropped > 0 ? ` drop ${stats.framesDropped}` : '';
    return ` ghostwire | ${size} | ${fps} fps | rtt ${rtt}${dropped} | ctrl+q quit`;
}
function concatBytes(a, b) {
    const out = new Uint8Array(a.length + b.length);
    out.set(a, 0);
    out.set(b, a.length);
    return out;
}
//# sourceMappingURL=terminal-viewer.js.map