export interface CompressorPort {
  readonly name: 'deflate' | 'none';
  compress(data: Uint8Array): Promise<Buffer>;
  decompress(data: Uint8Array, expectedLength: number): Promise<Uint8Array>;
}
