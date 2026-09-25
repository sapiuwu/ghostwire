import koffi from 'koffi';
import { GhostwireError } from '../../../core/domain/errors.js';
const SRCCOPY = 0x00cc0020;
const DIB_RGB_COLORS = 0;
const SM_CXSCREEN = 0;
const SM_CYSCREEN = 1;
let cachedApi = null;
let dpiAware = false;
function loadGdi() {
    if (cachedApi)
        return cachedApi;
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
    const StretchBlt = gdi32.func('int __stdcall StretchBlt(HDC dst, int dx, int dy, int dw, int dh, HDC src, int sx, int sy, int sw, int sh, uint32_t rop)');
    const GetDIBits = gdi32.func('int __stdcall GetDIBits(HDC hdc, HBITMAP hbm, uint32_t start, uint32_t lines, _Out_ void *bits, BITMAPINFO *bmi, uint32_t usage)');
    cachedApi = {
        getSystemMetrics: (index) => GetSystemMetrics(index),
        setProcessDpiAware: () => {
            if (dpiAware)
                return;
            SetProcessDPIAware();
            dpiAware = true;
        },
        getDC: () => GetDC(null),
        releaseDC: (hdc) => ReleaseDC(null, hdc),
        createCompatibleDC: (hdc) => CreateCompatibleDC(hdc),
        createCompatibleBitmap: (hdc, width, height) => CreateCompatibleBitmap(hdc, width, height),
        selectObject: (hdc, obj) => SelectObject(hdc, obj),
        deleteObject: (obj) => DeleteObject(obj),
        deleteDC: (hdc) => DeleteDC(hdc),
        bitBlt: (dst, dx, dy, w, h, src, sx, sy, rop) => BitBlt(dst, dx, dy, w, h, src, sx, sy, rop),
        stretchBlt: (dst, dx, dy, dw, dh, src, sx, sy, sw, sh, rop) => StretchBlt(dst, dx, dy, dw, dh, src, sx, sy, sw, sh, rop),
        getDIBits: (hdc, hbm, start, lines, bits, bmi, usage) => GetDIBits(hdc, hbm, start, lines, bits, bmi, usage),
        bitmapInfoHeaderSize: koffi.sizeof(BITMAPINFOHEADER),
    };
    return cachedApi;
}
function targetSize(screenW, screenH, maxWidth) {
    if (maxWidth <= 0 || screenW <= maxWidth)
        return { w: screenW, h: screenH };
    const w = Math.max(1, Math.floor(maxWidth));
    const h = Math.max(1, Math.round((screenH * w) / screenW));
    return { w, h };
}
export class GdiCapturer {
    maxWidth = 0;
    started = false;
    screenW = 0;
    screenH = 0;
    frameW = 0;
    frameH = 0;
    screenDC = 0n;
    memDC = 0n;
    bitmap = 0n;
    previousObject = 0n;
    buffers = [];
    activeBuffer = 0;
    bitmapInfo = {};
    async start(options) {
        const api = loadGdi();
        api.setProcessDpiAware();
        this.maxWidth = options.maxWidth;
        this.started = true;
        try {
            this.ensureTargets();
        }
        catch (err) {
            this.started = false;
            this.releaseTargets();
            throw err;
        }
    }
    bounds() {
        const api = loadGdi();
        return {
            width: api.getSystemMetrics(SM_CXSCREEN),
            height: api.getSystemMetrics(SM_CYSCREEN),
        };
    }
    grab() {
        if (!this.started) {
            throw new GhostwireError('internal', 'capturer has not been started');
        }
        const api = loadGdi();
        this.ensureTargets();
        const blitOk = this.frameW < this.screenW
            ? api.stretchBlt(this.memDC, 0, 0, this.frameW, this.frameH, this.screenDC, 0, 0, this.screenW, this.screenH, SRCCOPY)
            : api.bitBlt(this.memDC, 0, 0, this.frameW, this.frameH, this.screenDC, 0, 0, SRCCOPY);
        if (!blitOk) {
            throw new GhostwireError('internal', 'screen blit failed');
        }
        const buffer = this.buffers[this.activeBuffer];
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
    async close() {
        this.started = false;
        this.releaseTargets();
    }
    ensureTargets() {
        const api = loadGdi();
        const screen = api.getSystemMetrics(SM_CXSCREEN);
        const screenHeight = api.getSystemMetrics(SM_CYSCREEN);
        if (screen <= 0 || screenHeight <= 0) {
            throw new GhostwireError('internal', 'invalid screen metrics');
        }
        const target = targetSize(screen, screenHeight, this.maxWidth);
        if (this.started &&
            screen === this.screenW &&
            screenHeight === this.screenH &&
            target.w === this.frameW &&
            target.h === this.frameH) {
            return;
        }
        this.releaseTargets();
        this.screenW = screen;
        this.screenH = screenHeight;
        this.frameW = target.w;
        this.frameH = target.h;
        this.screenDC = api.getDC();
        if (!this.screenDC)
            throw new GhostwireError('internal', 'GetDC failed');
        this.memDC = api.createCompatibleDC(this.screenDC);
        if (!this.memDC)
            throw new GhostwireError('internal', 'CreateCompatibleDC failed');
        this.bitmap = api.createCompatibleBitmap(this.screenDC, this.frameW, this.frameH);
        if (!this.bitmap)
            throw new GhostwireError('internal', 'CreateCompatibleBitmap failed');
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
    releaseTargets() {
        const api = cachedApi;
        if (!api)
            return;
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
//# sourceMappingURL=gdi-capturer.js.map