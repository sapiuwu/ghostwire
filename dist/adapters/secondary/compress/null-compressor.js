import { GhostwireError } from '../../../core/domain/errors.js';
export class NullCompressor {
    name = 'none';
    compress(data) {
        return Promise.resolve(Buffer.from(data.buffer, data.byteOffset, data.byteLength));
    }
    decompress(_data, _expectedLength) {
        return Promise.reject(new GhostwireError('internal', 'this compressor does not decompress'));
    }
}
//# sourceMappingURL=null-compressor.js.map