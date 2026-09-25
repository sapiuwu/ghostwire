import { isLevelEnabled } from '../../../core/ports/outbound/logger.js';
const levelTag = {
    debug: 'DEBUG',
    info: 'INFO ',
    warn: 'WARN ',
    error: 'ERROR',
};
export class ConsoleLogger {
    level;
    sink;
    constructor(level = 'info', sink) {
        this.level = level;
        this.sink = sink ?? process.stderr;
    }
    debug(message, meta) {
        this.log('debug', message, meta);
    }
    info(message, meta) {
        this.log('info', message, meta);
    }
    warn(message, meta) {
        this.log('warn', message, meta);
    }
    error(message, meta) {
        this.log('error', message, meta);
    }
    log(level, message, meta) {
        if (!isLevelEnabled(this.level, level))
            return;
        const time = new Date().toISOString().slice(11, 23);
        let line = `${time} ${levelTag[level]} ${message}`;
        if (meta && Object.keys(meta).length > 0) {
            line += ` ${JSON.stringify(meta)}`;
        }
        this.sink.write(`${line}\n`);
    }
}
//# sourceMappingURL=console-logger.js.map