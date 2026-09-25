import { GhostwireError } from '../../../core/domain/errors.js';
import type { CompressorPort } from '../../../core/ports/outbound/compress.js';

export class NullCompressor implements CompressorPort {
  readonly name = 'none' as const;

  compress(data: Uint8Array): Promise<Buffer> {
    return Promise.resolve(Buffer.from(data.buffer, data.byteOffset, data.byteLength));
  }

  decompress(_data: Uint8Array, _expectedLength: number): Promise<Uint8Array> {
    return Promise.reject(new GhostwireError('internal', 'this compressor does not decompress'));
  }
}
