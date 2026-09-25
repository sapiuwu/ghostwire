import { isLevelEnabled } from '../../../core/ports/outbound/logger.js';
import type { LogLevel, Logger } from '../../../core/ports/outbound/logger.js';

const levelTag: Record<LogLevel, string> = {
  debug: 'DEBUG',
  info: 'INFO ',
  warn: 'WARN ',
  error: 'ERROR',
};

export class ConsoleLogger implements Logger {
  readonly level: LogLevel;
  private readonly sink: { write(chunk: string): boolean };

  constructor(level: LogLevel = 'info', sink?: { write(chunk: string): boolean }) {
    this.level = level;
    this.sink = sink ?? process.stderr;
  }

  debug(message: string, meta?: Record<string, unknown>): void {
    this.log('debug', message, meta);
  }

  info(message: string, meta?: Record<string, unknown>): void {
    this.log('info', message, meta);
  }

  warn(message: string, meta?: Record<string, unknown>): void {
    this.log('warn', message, meta);
  }

  error(message: string, meta?: Record<string, unknown>): void {
    this.log('error', message, meta);
  }

  private log(level: LogLevel, message: string, meta?: Record<string, unknown>): void {
    if (!isLevelEnabled(this.level, level)) return;
    const time = new Date().toISOString().slice(11, 23);
    let line = `${time} ${levelTag[level]} ${message}`;
    if (meta && Object.keys(meta).length > 0) {
      line += ` ${JSON.stringify(meta)}`;
    }
    this.sink.write(`${line}\n`);
  }
}
