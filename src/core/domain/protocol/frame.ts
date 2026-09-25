import { GhostwireError } from '../errors.js';
import { FRAME_META_SIZE, FrameEncoding, MAX_FRAME_RAW_SIZE, PixelFormat } from './constants.js';

export interface FrameMeta {
  seq: number;
  timestampMs: number;
  x: number;
  y: number;
  width: number;
  height: number;
  pixelFormat: number;
  encoding: number;
  rawLength: number;
}

export function encodeFrameMeta(meta: FrameMeta): Uint8Array {
  const buf = new Uint8Array(FRAME_META_SIZE);
  const view = new DataView(buf.buffer);
  view.setUint32(0, meta.seq >>> 0);
  view.setBigUint64(4, BigInt(meta.timestampMs));
  view.setUint16(12, meta.x);
  view.setUint16(14, meta.y);
  view.setUint16(16, meta.width);
  view.setUint16(18, meta.height);
  view.setUint8(20, meta.pixelFormat);
  view.setUint8(21, meta.encoding);
  view.setUint32(22, meta.rawLength >>> 0);
  return buf;
}

export function decodeFrameMeta(data: Uint8Array, offset = 0): FrameMeta {
  if (data.length - offset < FRAME_META_SIZE) {
    throw new GhostwireError('protocol', 'truncated frame header');
  }
  const view = new DataView(data.buffer, data.byteOffset + offset, FRAME_META_SIZE);
  const meta: FrameMeta = {
    seq: view.getUint32(0),
    timestampMs: Number(view.getBigUint64(4)),
    x: view.getUint16(12),
    y: view.getUint16(14),
    width: view.getUint16(16),
    height: view.getUint16(18),
    pixelFormat: view.getUint8(20),
    encoding: view.getUint8(21),
    rawLength: view.getUint32(22),
  };
  if (meta.pixelFormat !== PixelFormat.BGRA8) {
    throw new GhostwireError('protocol', `unsupported pixel format ${meta.pixelFormat}`);
  }
  if (meta.encoding !== FrameEncoding.Raw && meta.encoding !== FrameEncoding.Deflate) {
    throw new GhostwireError('protocol', `unsupported frame encoding ${meta.encoding}`);
  }
  if (meta.width <= 0 || meta.height <= 0) {
    throw new GhostwireError('protocol', 'invalid frame dimensions');
  }
  const expected = meta.width * meta.height * 4;
  if (meta.rawLength !== expected) {
    throw new GhostwireError('protocol', `frame raw length ${meta.rawLength} does not match ${expected}`);
  }
  if (meta.rawLength > MAX_FRAME_RAW_SIZE) {
    throw new GhostwireError('protocol', 'frame raw length exceeds limit');
  }
  return meta;
}
