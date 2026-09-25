import { deflateSync, inflateSync } from 'node:zlib';
import { GhostwireError } from '../errors.js';
import {
  HEADER_FLAG,
  HEADER_SIZE,
  MAX_PAYLOAD_SIZE,
  PROTOCOL_MAGIC,
  PROTOCOL_VERSION,
} from './constants.js';

export interface DecodedMessage {
  kind: number;
  flags: number;
  payload: Uint8Array;
}

const CONTROL_COMPRESS_THRESHOLD = 256;

export function encodeMessage(kind: number, payload: Uint8Array, flags = 0): Buffer {
  if (payload.length > MAX_PAYLOAD_SIZE) {
    throw new GhostwireError('protocol', `payload of ${payload.length} bytes exceeds limit`);
  }
  const header = Buffer.allocUnsafe(HEADER_SIZE);
  header.writeUInt32BE(PROTOCOL_MAGIC, 0);
  header.writeUInt8(PROTOCOL_VERSION, 4);
  header.writeUInt8(kind, 5);
  header.writeUInt16BE(flags, 6);
  header.writeUInt32BE(payload.length, 8);
  const body = Buffer.from(payload.buffer, payload.byteOffset, payload.length);
  return Buffer.concat([header, body]);
}

export function encodeJSONMessage(kind: number, value: unknown): Buffer {
  const json = Buffer.from(JSON.stringify(value), 'utf8');
  if (json.length > CONTROL_COMPRESS_THRESHOLD) {
    const compressed = deflateSync(json, { level: 3 });
    if (compressed.length < json.length) {
      return encodeMessage(kind, compressed, HEADER_FLAG.Deflate);
    }
  }
  return encodeMessage(kind, json);
}

export function decodeControlPayload(message: DecodedMessage): Uint8Array {
  if ((message.flags & HEADER_FLAG.Deflate) === 0) return message.payload;
  try {
    const out = inflateSync(Buffer.from(message.payload.buffer, message.payload.byteOffset, message.payload.byteLength), {
      maxOutputLength: 1 << 20,
    });
    return new Uint8Array(out.buffer, out.byteOffset, out.byteLength);
  } catch (err) {
    throw new GhostwireError('protocol', 'failed to inflate control payload', { cause: err });
  }
}

export class MessageDecoder {
  private chunks: Buffer[] = [];
  private head = 0;
  private buffered = 0;

  feed(chunk: Uint8Array): DecodedMessage[] {
    const buffer =
      chunk instanceof Buffer
        ? chunk
        : Buffer.from(chunk.buffer, chunk.byteOffset, chunk.byteLength);
    if (buffer.length > 0) {
      this.chunks.push(buffer);
      this.buffered += buffer.length;
    }
    const messages: DecodedMessage[] = [];
    for (;;) {
      if (this.buffered < HEADER_SIZE) break;
      const header = this.peek(HEADER_SIZE);
      if (header.readUInt32BE(0) !== PROTOCOL_MAGIC) {
        this.reset();
        throw new GhostwireError('protocol', 'stream is out of sync (bad magic)');
      }
      const version = header.readUInt8(4);
      if (version !== PROTOCOL_VERSION) {
        this.reset();
        throw new GhostwireError('version', `unsupported protocol version ${version}`);
      }
      const kind = header.readUInt8(5);
      const flags = header.readUInt16BE(6);
      const length = header.readUInt32BE(8);
      if (length > MAX_PAYLOAD_SIZE) {
        this.reset();
        throw new GhostwireError('protocol', `payload length ${length} exceeds limit`);
      }
      if (this.buffered < HEADER_SIZE + length) break;
      this.skip(HEADER_SIZE);
      const payload = this.take(length);
      messages.push({ kind, flags, payload });
    }
    return messages;
  }

  reset(): void {
    this.chunks = [];
    this.head = 0;
    this.buffered = 0;
  }

  private peek(count: number): Buffer {
    const first = this.chunks[0]!;
    if (first.length - this.head >= count) {
      return first.subarray(this.head, this.head + count);
    }
    const parts: Buffer[] = [];
    let need = count;
    let index = 0;
    while (need > 0) {
      const chunk = this.chunks[index]!;
      const start = index === 0 ? this.head : 0;
      const available = chunk.length - start;
      const take = Math.min(need, available);
      parts.push(chunk.subarray(start, start + take));
      need -= take;
      index++;
    }
    return Buffer.concat(parts);
  }

  private skip(count: number): void {
    this.head += count;
    this.buffered -= count;
    this.compact();
  }

  private take(count: number): Uint8Array {
    const first = this.chunks[0]!;
    if (first.length - this.head >= count) {
      const out = first.subarray(this.head, this.head + count);
      this.head += count;
      this.buffered -= count;
      this.compact();
      return out;
    }
    const parts: Buffer[] = [];
    let need = count;
    while (need > 0) {
      const chunk = this.chunks[0]!;
      const available = chunk.length - this.head;
      const take = Math.min(need, available);
      parts.push(chunk.subarray(this.head, this.head + take));
      need -= take;
      this.buffered -= take;
      this.head += take;
      if (this.head === chunk.length) {
        this.chunks.shift();
        this.head = 0;
      }
    }
    this.compact();
    return Buffer.concat(parts);
  }

  private compact(): void {
    while (this.chunks.length > 0 && this.head >= this.chunks[0]!.length) {
      this.chunks.shift();
      this.head = 0;
    }
  }
}
