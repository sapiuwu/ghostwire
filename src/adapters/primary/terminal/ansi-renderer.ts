import type { ScreenFrame } from '../../../core/domain/screen/frame.js';

export interface RendererIo {
  write(chunk: string): void;
}

const HALF_BLOCK = '\u2580';

export class AnsiRenderer {
  private renderRows: number;
  private curFg: Int32Array;
  private curBg: Int32Array;
  private nextFg: Int32Array;
  private nextBg: Int32Array;
  private sgrFg = -1;
  private sgrBg = -1;

  constructor(
    private io: RendererIo,
    public cols: number,
    public rows: number,
    private statusEnabled = true,
  ) {
    this.renderRows = this.computeRenderRows();
    const size = this.cols * this.renderRows;
    this.curFg = new Int32Array(size).fill(-1);
    this.curBg = new Int32Array(size).fill(-1);
    this.nextFg = new Int32Array(size);
    this.nextBg = new Int32Array(size);
  }

  private computeRenderRows(): number {
    return Math.max(1, this.statusEnabled ? this.rows - 1 : this.rows);
  }

  get viewportRows(): number {
    return this.renderRows;
  }

  begin(): void {
    this.io.write(
      '\x1b[?25l\x1b[?7l' +
        '\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h' +
        '\x1b[2J\x1b[H',
    );
    this.curFg.fill(-1);
    this.curBg.fill(-1);
    this.sgrFg = -1;
    this.sgrBg = -1;
  }

  resize(cols: number, rows: number): void {
    if (cols === this.cols && rows === this.rows) return;
    this.cols = cols;
    this.rows = rows;
    this.renderRows = this.computeRenderRows();
    const size = this.cols * this.renderRows;
    this.curFg = new Int32Array(size).fill(-1);
    this.curBg = new Int32Array(size).fill(-1);
    this.nextFg = new Int32Array(size);
    this.nextBg = new Int32Array(size);
    this.io.write('\x1b[2J\x1b[H');
  }

  render(frame: ScreenFrame): void {
    const cols = this.cols;
    const renderRows = this.renderRows;
    const nextFg = this.nextFg;
    const nextBg = this.nextBg;
    const source = frame.data;
    const frameWidth = frame.width;
    const frameHeight = frame.height;

    for (let row = 0; row < renderRows; row++) {
      const y0 = Math.floor((row * frameHeight) / renderRows);
      const y1 = Math.max(y0 + 1, Math.floor(((row + 1) * frameHeight) / renderRows));
      const mid = Math.min(y1, Math.max(y0 + 1, Math.floor((y0 + y1) / 2)));

      for (let col = 0; col < cols; col++) {
        const x0 = Math.floor((col * frameWidth) / cols);
        const x1 = Math.max(x0 + 1, Math.floor(((col + 1) * frameWidth) / cols));
        const top = averageColor(source, x0, y0, x1, mid, frameWidth);
        const bottom =
          mid >= y1
            ? top
            : averageColor(source, x0, mid, x1, y1, frameWidth);
        const index = row * cols + col;
        nextFg[index] = top;
        nextBg[index] = bottom;
      }
    }

    let out = '';
    for (let row = 0; row < renderRows; row++) {
      const rowBase = row * cols;
      let col = 0;
      while (col < cols) {
        const index = rowBase + col;
        if (nextFg[index] === this.curFg[index] && nextBg[index] === this.curBg[index]) {
          col++;
          continue;
        }
        out += `\x1b[${row + 1};${col + 1}H`;
        while (col < cols) {
          const i = rowBase + col;
          const fg = nextFg[i]!;
          const bg = nextBg[i]!;
          if (fg === this.curFg[i] && bg === this.curBg[i]) break;
          out += this.sgr(fg, bg);
          out += HALF_BLOCK;
          this.curFg[i] = fg;
          this.curBg[i] = bg;
          col++;
        }
      }
    }
    if (out !== '') this.io.write(out);
  }

  status(text: string): void {
    if (!this.statusEnabled) return;
    const row = this.rows;
    this.io.write(
      `\x1b[${row};1H\x1b[2K\x1b[38;2;170;187;204m${text}\x1b[0m`,
    );
    this.sgrFg = -1;
    this.sgrBg = -1;
  }

  end(): void {
    this.io.write(
      '\x1b[?1006l\x1b[?1003l\x1b[?1002l\x1b[?1000l' +
        '\x1b[0m\x1b[?25h\x1b[?7h\x1b[2J\x1b[H',
    );
  }

  private sgr(fg: number, bg: number): string {
    let out = '';
    if (fg !== this.sgrFg) {
      out += `\x1b[38;2;${(fg >> 16) & 0xff};${(fg >> 8) & 0xff};${fg & 0xff}m`;
      this.sgrFg = fg;
    }
    if (bg !== this.sgrBg) {
      out += `\x1b[48;2;${(bg >> 16) & 0xff};${(bg >> 8) & 0xff};${bg & 0xff}m`;
      this.sgrBg = bg;
    }
    return out;
  }
}

function averageColor(
  pixels: Uint8Array,
  x0: number,
  y0: number,
  x1: number,
  y1: number,
  stride: number,
): number {
  let sumB = 0;
  let sumG = 0;
  let sumR = 0;
  let count = 0;
  for (let y = y0; y < y1; y++) {
    let offset = (y * stride + x0) * 4;
    for (let x = x0; x < x1; x++) {
      sumB += pixels[offset]!;
      sumG += pixels[offset + 1]!;
      sumR += pixels[offset + 2]!;
      count++;
      offset += 4;
    }
  }
  if (count === 0) return 0;
  const r = Math.round(sumR / count);
  const g = Math.round(sumG / count);
  const b = Math.round(sumB / count);
  return (r << 16) | (g << 8) | b;
}
