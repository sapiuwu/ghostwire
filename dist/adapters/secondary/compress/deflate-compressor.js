import { deflate, inflate } from 'node:zlib';
import { GhostwireError } from '../../../core/domain/errors.js';
export class DeflateCompressor {
    name = 'deflate';
    level;
    constructor(level = 1) {
        this.level = level;
    }
    compress(data) {
        return new Promise((resolve, reject) => {
            deflate(data, { level: this.level, memLevel: 8 }, (err, result) => {
                if (err)
                    reject(new GhostwireError('internal', `deflate failed: ${err.message}`, { cause: err }));
                else
                    resolve(result);
            });
        });
    }
    decompress(data, expectedLength) {
        return new Promise((resolve, reject) => {
            inflate(data, { maxOutputLength: expectedLength }, (err, result) => {
                if (err) {
                    reject(new GhostwireError('protocol', `inflate failed: ${err.message}`, { cause: err }));
                    return;
                }
                if (result.length !== expectedLength) {
                    reject(new GhostwireError('protocol', `inflated size ${result.length} does not match expected ${expectedLength}`));
                    return;
                }
                resolve(new Uint8Array(result.buffer, result.byteOffset, result.byteLength));
            });
        });
    }
}
//# sourceMappingURL=deflate-compressor.js.map