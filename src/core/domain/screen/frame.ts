export interface ScreenSize {
  width: number;
  height: number;
}

export type PixelFormatName = 'bgra8';

export interface ScreenFrame {
  readonly width: number;
  readonly height: number;
  readonly format: PixelFormatName;
  readonly timestampMs: number;
  readonly data: Uint8Array;
}

export function frameByteLength(width: number, height: number): number {
  return width * height * 4;
}

export function createScreenFrame(width: number, height: number, timestampMs: number): ScreenFrame {
  return {
    width,
    height,
    format: 'bgra8',
    timestampMs,
    data: new Uint8Array(frameByteLength(width, height)),
  };
}

export function scaleFrameNearest(src: ScreenFrame, dstWidth: number, dstHeight: number): ScreenFrame {
  if (dstWidth === src.width && dstHeight === src.height) return src;
  const dst = createScreenFrame(dstWidth, dstHeight, src.timestampMs);
  const srcData = src.data;
  const dstData = dst.data;
  for (let y = 0; y < dstHeight; y++) {
    const sy = Math.min(src.height - 1, Math.floor((y * src.height) / dstHeight));
    const srcRow = sy * src.width * 4;
    const dstRow = y * dstWidth * 4;
    for (let x = 0; x < dstWidth; x++) {
      const sx = Math.min(src.width - 1, Math.floor((x * src.width) / dstWidth));
      const si = srcRow + sx * 4;
      const di = dstRow + x * 4;
      dstData[di] = srcData[si]!;
      dstData[di + 1] = srcData[si + 1]!;
      dstData[di + 2] = srcData[si + 2]!;
      dstData[di + 3] = srcData[si + 3]!;
    }
  }
  return dst;
}

export function samePixels(a: Uint8Array, b: Uint8Array): boolean {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  const limit = a.length;
  let i = 0;
  const chunk = 8192;
  while (i < limit) {
    const end = Math.min(limit, i + chunk);
    for (let j = i; j < end; j++) {
      if (a[j] !== b[j]) return false;
    }
    i = end;
  }
  return true;
}
