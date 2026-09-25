import koffi from 'koffi';
import { GhostwireError } from '../../../core/domain/errors.js';
import type { ScreenFrame, ScreenSize } from '../../../core/domain/screen/frame.js';
import type { CaptureOptions, ScreenCapturerPort } from '../../../core/ports/outbound/capture.js';

const SRCCOPY = 0x00cc0020;
const DIB_RGB_COLORS = 0;
const SM_CXSCREEN = 0;
const SM_CYSCREEN = 1;

interface GdiApi {
  getSystemMetrics(index: number): number;
  setProcessDpiAware(): void;
  getDC(): bigint;
  releaseDC(hdc: bigint): number;
  createCompatibleDC(hdc: bigint): bigint;
  createCompatibleBitmap(hdc: bigint, width: number, height: number): bigint;
  selectObject(hdc: bigint, obj: bigint): bigint;
  deleteObject(obj: bigint): number;
  deleteDC(hdc: bigint): number;
  bitBlt(dst: bigint, dx: number, dy: number, w: number, h: number, src: bigint, sx: number, sy: number, rop: number): number;
  stretchBlt(
    dst: bigint,
    dx: number,
    dy: number,
    dw: number,
    dh: number,
    src: bigint,
    sx: number,
    sy: number,
    sw: number,
    sh: number,
    rop: number,
  ): number;
  getDIBits(hdc: bigint, hbm: bigint, start: number, lines: number, bits: Uint8Array, bmi: Record<string, unknown>, usage: number): number;
  bitmapInfoHeaderSize: number;
}

let cachedApi: GdiApi | null = null;
let dpiAware = false;

function loadGdi(): GdiApi {
  if (cachedApi) return cachedApi;
  if (process.platform !== 'win32') {
    throw new GhostwireError('unsupported', 'screen capture requires Windows (GDI)');
  }

  const user32 = koffi.load('user32.dll');
  const gdi32 = koffi.load('gdi32.dll');

  const HWND = koffi.pointer('HWND', koffi.opaque());
  const HDC = koffi.pointer('HDC', koffi.opaque());
  const HBITMAP = koffi.pointer('HBITMAP', koffi.opaque());
  const HGDIOBJ = koffi.pointer('HGDIOBJ', koffi.opaque());

  const BITMAPINFOHEADER = koffi.struct('BITMAPINFOHEADER', {
    biSize: 'uint32_t',
    biWidth: 'int32_t',
    biHeight: 'int32_t',
    biPlanes: 'uint16_t',
    biBitCount: 'uint16_t',
    biCompression: 'uint32_t',
    biSizeImage: 'uint32_t',
    biXPelsPerMeter: 'int32_t',
    biYPelsPerMeter: 'int32_t',
    biClrUsed: 'uint32_t',
    biClrImportant: 'uint32_t',
  });
  const BITMAPINFO = koffi.struct('BITMAPINFO', {
    bmiHeader: BITMAPINFOHEADER,
    bmiColors: koffi.array('uint32_t', 1),
  });

  const GetSystemMetrics = user32.func('int __stdcall GetSystemMetrics(int index)');
  const SetProcessDPIAware = user32.func('int __stdcall SetProcessDPIAware()');
  const GetDC = user32.func('HDC __stdcall GetDC(HWND hWnd)');
  const ReleaseDC = user32.func('int __stdcall ReleaseDC(HWND hWnd, HDC hdc)');
  const CreateCompatibleDC = gdi32.func('HDC __stdcall CreateCompatibleDC(HDC hdc)');
  const CreateCompatibleBitmap = gdi32.func('HBITMAP __stdcall CreateCompatibleBitmap(HDC hdc, int w, int h)');
  const SelectObject = gdi32.func('HGDIOBJ __stdcall SelectObject(HDC hdc, HGDIOBJ obj)');
  const DeleteObject = gdi32.func('int __stdcall DeleteObject(HGDIOBJ obj)');
  const DeleteDC = gdi32.func('int __stdcall DeleteDC(HDC hdc)');
  const BitBlt = gdi32.func('int __stdcall BitBlt(HDC dst, int dx, int dy, int w, int h, HDC src, int sx, int sy, uint32_t rop)');
  const StretchBlt = gdi32.func(
    'int __stdcall StretchBlt(HDC dst, int dx, int dy, int dw, int dh, HDC src, int sx, int sy, int sw, int sh, uint32_t rop)',
  );
  const GetDIBits = gdi32.func(
    'int __stdcall GetDIBits(HDC hdc, HBITMAP hbm, uint32_t start, uint32_t lines, _Out_ void *bits, BITMAPINFO *bmi, uint32_t usage)',
  );

  cachedApi = {
    getSystemMetrics: (index) => GetSystemMetrics(index) as number,
    setProcessDpiAware: () => {
      if (dpiAware) return;
      SetProcessDPIAware();
      dpiAware = true;
    },
    getDC: () => GetDC(null) as bigint,
    releaseDC: (hdc) => ReleaseDC(null, hdc) as number,
    createCompatibleDC: (hdc) => CreateCompatibleDC(hdc) as bigint,
    createCompatibleBitmap: (hdc, width, height) => CreateCompatibleBitmap(hdc, width, height) as bigint,
    selectObject: (hdc, obj) => SelectObject(hdc, obj) as bigint,
    deleteObject: (obj) => DeleteObject(obj) as number,
    deleteDC: (hdc) => DeleteDC(hdc) as number,
    bitBlt: (dst, dx, dy, w, h, src, sx, sy, rop) => BitBlt(dst, dx, dy, w, h, src, sx, sy, rop) as number,
    stretchBlt: (dst, dx, dy, dw, dh, src, sx, sy, sw, sh, rop) =>
      StretchBlt(dst, dx, dy, dw, dh, src, sx, sy, sw, sh, rop) as number,
    getDIBits: (hdc, hbm, start, lines, bits, bmi, usage) =>
      GetDIBits(hdc, hbm, start, lines, bits, bmi, usage) as number,
    bitmapInfoHeaderSize: koffi.sizeof(BITMAPINFOHEADER),
  };
  return cachedApi;
}

function targetSize(screenW: number, screenH: number, maxWidth: number): { w: number; h: number } {
  if (maxWidth <= 0 || screenW <= maxWidth) return { w: screenW, h: screenH };
  const w = Math.max(1, Math.floor(maxWidth));
  const h = Math.max(1, Math.round((screenH * w) / screenW));
  return { w, h };
}

export class GdiCapturer implements ScreenCapturerPort {
  private maxWidth = 0;
  private started = false;
  private screenW = 0;
  private screenH = 0;
  private frameW = 0;
  private frameH = 0;
  private screenDC = 0n;
  private memDC = 0n;
  private bitmap = 0n;
  private previousObject = 0n;
  private buffers: Uint8Array[] = [];
  private activeBuffer = 0;
  private bitmapInfo: Record<string, unknown> = {};

  async start(options: CaptureOptions): Promise<void> {
    const api = loadGdi();
    api.setProcessDpiAware();
    this.maxWidth = options.maxWidth;
    this.started = true;
    try {
      this.ensureTargets();
    } catch (err) {
      this.started = false;
      this.releaseTargets();
      throw err;
    }
  }

  bounds(): ScreenSize {
    const api = loadGdi();
    return {
      width: api.getSystemMetrics(SM_CXSCREEN),
      height: api.getSystemMetrics(SM_CYSCREEN),
    };
  }

  grab(): ScreenFrame {
    if (!this.started) {
      throw new GhostwireError('internal', 'capturer has not been started');
    }
    const api = loadGdi();
    this.ensureTargets();
    const blitOk =
      this.frameW < this.screenW
        ? api.stretchBlt(this.memDC, 0, 0, this.frameW, this.frameH, this.screenDC, 0, 0, this.screenW, this.screenH, SRCCOPY)
        : api.bitBlt(this.memDC, 0, 0, this.frameW, this.frameH, this.screenDC, 0, 0, SRCCOPY);
    if (!blitOk) {
      throw new GhostwireError('internal', 'screen blit failed');
    }
    const buffer = this.buffers[this.activeBuffer]!;
    const lines = api.getDIBits(this.memDC, this.bitmap, 0, this.frameH, buffer, this.bitmapInfo, DIB_RGB_COLORS);
    if (lines === 0) {
      throw new GhostwireError('internal', 'GetDIBits failed');
    }
    this.activeBuffer = this.activeBuffer === 0 ? 1 : 0;
    return {
      width: this.frameW,
      height: this.frameH,
      format: 'bgra8',
      timestampMs: Date.now(),
      data: buffer,
    };
  }

  async close(): Promise<void> {
    this.started = false;
    this.releaseTargets();
  }

  private ensureTargets(): void {
    const api = loadGdi();
    const screen = api.getSystemMetrics(SM_CXSCREEN);
    const screenHeight = api.getSystemMetrics(SM_CYSCREEN);
    if (screen <= 0 || screenHeight <= 0) {
      throw new GhostwireError('internal', 'invalid screen metrics');
    }
    const target = targetSize(screen, screenHeight, this.maxWidth);
    if (
      this.started &&
      screen === this.screenW &&
      screenHeight === this.screenH &&
      target.w === this.frameW &&
      target.h === this.frameH
    ) {
      return;
    }
    this.releaseTargets();
    this.screenW = screen;
    this.screenH = screenHeight;
    this.frameW = target.w;
    this.frameH = target.h;
    this.screenDC = api.getDC();
    if (!this.screenDC) throw new GhostwireError('internal', 'GetDC failed');
    this.memDC = api.createCompatibleDC(this.screenDC);
    if (!this.memDC) throw new GhostwireError('internal', 'CreateCompatibleDC failed');
    this.bitmap = api.createCompatibleBitmap(this.screenDC, this.frameW, this.frameH);
    if (!this.bitmap) throw new GhostwireError('internal', 'CreateCompatibleBitmap failed');
    this.previousObject = api.selectObject(this.memDC, this.bitmap);
    this.buffers = [
      new Uint8Array(this.frameW * this.frameH * 4),
      new Uint8Array(this.frameW * this.frameH * 4),
    ];
    this.activeBuffer = 0;
    this.bitmapInfo = {
      bmiHeader: {
        biSize: api.bitmapInfoHeaderSize,
        biWidth: this.frameW,
        biHeight: -this.frameH,
        biPlanes: 1,
        biBitCount: 32,
        biCompression: 0,
        biSizeImage: 0,
        biXPelsPerMeter: 2835,
        biYPelsPerMeter: 2835,
        biClrUsed: 0,
        biClrImportant: 0,
      },
      bmiColors: [0],
    };
  }

  private releaseTargets(): void {
    const api = cachedApi;
    if (!api) return;
    if (this.memDC && this.bitmap) {
      api.selectObject(this.memDC, this.previousObject);
    }
    if (this.bitmap) {
      api.deleteObject(this.bitmap);
    }
    if (this.memDC) {
      api.deleteDC(this.memDC);
    }
    if (this.screenDC) {
      api.releaseDC(this.screenDC);
    }
    this.screenDC = 0n;
    this.memDC = 0n;
    this.bitmap = 0n;
    this.previousObject = 0n;
    this.buffers = [];
    this.frameW = 0;
    this.frameH = 0;
    this.screenW = 0;
    this.screenH = 0;
  }
}
