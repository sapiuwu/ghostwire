import { deflate, inflate } from 'node:zlib';
import { GhostwireError } from '../../../core/domain/errors.js';
import type { CompressorPort } from '../../../core/ports/outbound/compress.js';

export class DeflateCompressor implements CompressorPort {
  readonly name = 'deflate' as const;
  private readonly level: number;

  constructor(level = 1) {
    this.level = level;
  }

  compress(data: Uint8Array): Promise<Buffer> {
    return new Promise((resolve, reject) => {
      deflate(
        data,
        { level: this.level, memLevel: 8 },
        (err, result) => {
          if (err) reject(new GhostwireError('internal', `deflate failed: ${err.message}`, { cause: err }));
          else resolve(result);
        },
      );
    });
  }

  decompress(data: Uint8Array, expectedLength: number): Promise<Uint8Array> {
    return new Promise((resolve, reject) => {
      inflate(data, { maxOutputLength: expectedLength }, (err, result) => {
        if (err) {
          reject(new GhostwireError('protocol', `inflate failed: ${err.message}`, { cause: err }));
          return;
        }
        if (result.length !== expectedLength) {
          reject(
            new GhostwireError(
              'protocol',
              `inflated size ${result.length} does not match expected ${expectedLength}`,
            ),
          );
          return;
        }
        resolve(new Uint8Array(result.buffer, result.byteOffset, result.byteLength));
      });
    });
  }
}
