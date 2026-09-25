import { Key, Modifier, type InputEvent } from '../../../core/domain/input/events.js';

export interface TerminalParseOptions {
  mapCell(cellX: number, cellY: number): { x: number; y: number };
}

export interface TerminalParseResult {
  events: InputEvent[];
  consumed: number;
}

const PASTE_START = [0x1b, 0x5b, 0x32, 0x30, 0x30, 0x7e];
const PASTE_END = [0x1b, 0x5b, 0x32, 0x30, 0x31, 0x7e];

const SHIFTED_SYMBOLS: Record<string, number> = {
  '!': Key.Digit1,
  '@': Key.Digit2,
  '#': Key.Digit3,
  $: Key.Digit4,
  '%': Key.Digit5,
  '^': Key.Digit6,
  '&': Key.Digit7,
  '*': Key.Digit8,
  '(': Key.Digit9,
  ')': Key.Digit0,
  _: Key.OemMinus,
  '+': Key.OemPlus,
  '{': Key.Oem4,
  '}': Key.Oem6,
  '|': Key.Oem5,
  ':': Key.Oem1,
  '"': Key.Oem7,
  '<': Key.OemComma,
  '>': Key.OemPeriod,
  '?': Key.Oem2,
  '~': Key.Oem3,
};

const PLAIN_SYMBOLS: Record<string, number> = {
  '-': Key.OemMinus,
  '=': Key.OemPlus,
  '[': Key.Oem4,
  ']': Key.Oem6,
  '\\': Key.Oem5,
  ';': Key.Oem1,
  "'": Key.Oem7,
  ',': Key.OemComma,
  '.': Key.OemPeriod,
  '/': Key.Oem2,
  '`': Key.Oem3,
};

const CSI_KEYS: Record<string, number> = {
  A: Key.Up,
  B: Key.Down,
  C: Key.Right,
  D: Key.Left,
  H: Key.Home,
  F: Key.End,
};

const TILDE_KEYS: Record<number, number> = {
  1: Key.Home,
  2: Key.Insert,
  3: Key.Delete,
  4: Key.End,
  5: Key.PageUp,
  6: Key.PageDown,
  7: Key.Home,
  8: Key.End,
  11: Key.F1,
  12: Key.F2,
  13: Key.F3,
  14: Key.F4,
  15: Key.F5,
  17: Key.F6,
  18: Key.F7,
  19: Key.F8,
  20: Key.F9,
  21: Key.F10,
  23: Key.F11,
  24: Key.F12,
};

const SS3_KEYS: Record<string, number> = {
  P: Key.F1,
  Q: Key.F2,
  R: Key.F3,
  S: Key.F4,
  A: Key.Up,
  B: Key.Down,
  C: Key.Right,
  D: Key.Left,
  H: Key.Home,
  F: Key.End,
};

export function keyPair(key: number, modifiers: number): InputEvent[] {
  return [
    { kind: 'key', key, down: true, modifiers },
    { kind: 'key', key, down: false, modifiers },
  ];
}

export function keyFromChar(ch: string, baseModifiers = 0): { key: number; modifiers: number } | null {
  if (ch.length !== 1) return null;
  const code = ch.charCodeAt(0);
  if (ch >= 'a' && ch <= 'z') return { key: Key.A + (code - 97), modifiers: baseModifiers };
  if (ch >= 'A' && ch <= 'Z') return { key: Key.A + (code - 65), modifiers: baseModifiers | Modifier.Shift };
  if (ch >= '0' && ch <= '9') return { key: Key.Digit0 + (code - 48), modifiers: baseModifiers };
  if (ch === ' ') return { key: Key.Space, modifiers: baseModifiers };
  const shifted = SHIFTED_SYMBOLS[ch];
  if (shifted !== undefined) return { key: shifted, modifiers: baseModifiers | Modifier.Shift };
  const plain = PLAIN_SYMBOLS[ch];
  if (plain !== undefined) return { key: plain, modifiers: baseModifiers };
  return null;
}

export function mapCellToScreen(
  cellX: number,
  cellY: number,
  screen: { width: number; height: number },
  cols: number,
  rows: number,
): { x: number; y: number } {
  if (screen.width <= 0 || screen.height <= 0 || cols <= 0 || rows <= 0) return { x: 0, y: 0 };
  const col = Math.min(Math.max(Math.trunc(cellX), 1), cols);
  const row = Math.min(Math.max(Math.trunc(cellY), 1), rows);
  const x = Math.floor((col - 0.5) * (screen.width / cols));
  const y = Math.floor((2 * row - 0.5) * (screen.height / (2 * rows)));
  return {
    x: Math.min(x, screen.width - 1),
    y: Math.min(y, screen.height - 1),
  };
}

function decodeXtermMods(value: number | undefined): number {
  if (value === undefined || !Number.isFinite(value) || value < 1) return 0;
  const flags = Math.trunc(value) - 1;
  let mods = 0;
  if ((flags & 1) !== 0) mods |= Modifier.Shift;
  if ((flags & 2) !== 0) mods |= Modifier.Alt;
  if ((flags & 4) !== 0) mods |= Modifier.Ctrl;
  if ((flags & 8) !== 0) mods |= Modifier.Meta;
  return mods;
}

function pushText(events: InputEvent[], code: number): void {
  if (code <= 0xffff) {
    events.push({ kind: 'text', code });
    return;
  }
  const offset = code - 0x10000;
  events.push({ kind: 'text', code: 0xd800 + (offset >> 10) });
  events.push({ kind: 'text', code: 0xdc00 + (offset & 0x3ff) });
}

function ascii(buf: Uint8Array, from: number, to: number): string {
  let out = '';
  for (let i = from; i < to; i++) out += String.fromCharCode(buf[i]!);
  return out;
}

function matchesAt(buf: Uint8Array, index: number, seq: number[]): boolean {
  if (index + seq.length > buf.length) return false;
  for (let i = 0; i < seq.length; i++) {
    if (buf[index + i] !== seq[i]) return false;
  }
  return true;
}

function decodeUtf8(buf: Uint8Array, index: number, flush: boolean): { code: number; next: number } | null {
  const lead = buf[index]!;
  let length: number;
  if (lead >= 0xc2 && lead <= 0xdf) length = 2;
  else if (lead >= 0xe0 && lead <= 0xef) length = 3;
  else if (lead >= 0xf0 && lead <= 0xf4) length = 4;
  else return { code: -1, next: index + 1 };
  if (index + length > buf.length) {
    if (flush) return { code: -1, next: index + 1 };
    return null;
  }
  let code = lead & (0xff >> (length + 1));
  for (let i = 1; i < length; i++) {
    const byte = buf[index + i]!;
    if ((byte & 0xc0) !== 0x80) return { code: -1, next: index + 1 };
    code = (code << 6) | (byte & 0x3f);
  }
  if (code < 0x20 || code > 0x10ffff || (code >= 0xd800 && code <= 0xdfff)) {
    return { code: -1, next: index + 1 };
  }
  return { code, next: index + length };
}

function parsePlain(buf: Uint8Array, from: number, to: number, options: TerminalParseOptions, flush: boolean): TerminalParseResult {
  const events: InputEvent[] = [];
  let i = from;
  while (i < to) {
    const byte = buf[i]!;
    if (byte === 0x1b) {
      const parsed = parseEscape(buf, i, to, options, flush);
      if (!parsed) break;
      events.push(...parsed.events);
      i = parsed.next;
      continue;
    }
    if (byte === 0x7f || byte === 0x08) {
      events.push(...keyPair(Key.Backspace, 0));
      i++;
      continue;
    }
    if (byte === 0x09) {
      events.push(...keyPair(Key.Tab, 0));
      i++;
      continue;
    }
    if (byte === 0x0d || byte === 0x0a) {
      events.push(...keyPair(Key.Enter, 0));
      i++;
      continue;
    }
    if (byte === 0x00) {
      events.push(...keyPair(Key.Space, Modifier.Ctrl));
      i++;
      continue;
    }
    if (byte < 0x20) {
      if (byte >= 1 && byte <= 26) {
        events.push(...keyPair(Key.A + byte - 1, Modifier.Ctrl));
      }
      i++;
      continue;
    }
    if (byte < 0x80) {
      const mapped = keyFromChar(String.fromCharCode(byte), 0);
      if (mapped) events.push(...keyPair(mapped.key, mapped.modifiers));
      i++;
      continue;
    }
    const decoded = decodeUtf8(buf, i, flush);
    if (!decoded) break;
    if (decoded.code >= 0x20) pushText(events, decoded.code);
    i = decoded.next;
  }
  return { events, consumed: i };
}

function parseSgrMouse(params: string, press: boolean, options: TerminalParseOptions): InputEvent[] {
  const parts = params.slice(1).split(';');
  const buttonBits = Number(parts[0]);
  const cellX = Number(parts[1]);
  const cellY = Number(parts[2]);
  if (!Number.isFinite(buttonBits) || !Number.isFinite(cellX) || !Number.isFinite(cellY)) return [];
  let modifiers = 0;
  if ((buttonBits & 4) !== 0) modifiers |= Modifier.Shift;
  if ((buttonBits & 8) !== 0) modifiers |= Modifier.Alt;
  if ((buttonBits & 16) !== 0) modifiers |= Modifier.Ctrl;
  const pos = options.mapCell(cellX, cellY);
  if ((buttonBits & 64) !== 0) {
    const direction = buttonBits & 3;
    return [{ kind: 'wheel', x: pos.x, y: pos.y, deltaY: direction === 0 ? 120 : -120, modifiers }];
  }
  if ((buttonBits & 32) !== 0) {
    return [{ kind: 'mouseMove', x: pos.x, y: pos.y, modifiers }];
  }
  const button = buttonBits & 3;
  if (button === 3) {
    return press ? [{ kind: 'mouseMove', x: pos.x, y: pos.y, modifiers }] : [];
  }
  const mappedButton = button === 0 ? 0 : button === 1 ? 2 : 1;
  return [{ kind: 'mouseButton', button: mappedButton as 0 | 1 | 2, down: press, x: pos.x, y: pos.y, modifiers }];
}

function parseCsi(
  buf: Uint8Array,
  start: number,
  limit: number,
  options: TerminalParseOptions,
  flush: boolean,
): { events: InputEvent[]; next: number } | null {
  let i = start + 1;
  while (i < limit) {
    const byte = buf[i]!;
    if (byte >= 0x40 && byte <= 0x7e) break;
    if (i - start > 64) return { events: [], next: limit };
    i++;
  }
  if (i >= limit) {
    if (flush) return { events: [], next: limit };
    return null;
  }
  const final = String.fromCharCode(buf[i]!);
  const params = ascii(buf, start + 1, i);
  const next = i + 1;

  if (params.startsWith('<')) {
    const press = final === 'M';
    if (!press && final !== 'm') return { events: [], next };
    return { events: parseSgrMouse(params, press, options), next };
  }

  if (params === '200' && final === '~') {
    const end = findSequence(buf, i + 1, limit, PASTE_END);
    if (end < 0) {
      if (!flush) return null;
      const inner = parsePlain(buf, i + 1, limit, options, true);
      return { events: inner.events, next: limit };
    }
    const inner = parsePlain(buf, i + 1, end, options, true);
    return { events: inner.events, next: end + PASTE_END.length };
  }

  if ((final === 'I' || final === 'O') && params === '') return { events: [], next };
  if (final === 'Z') return { events: keyPair(Key.Tab, Modifier.Shift), next };

  const numbers = params === '' ? [] : params.split(';').map((part) => (part === '' ? NaN : Number(part)));
  const modifierValue = numbers.length > 1 ? numbers[1] : undefined;

  if (final in CSI_KEYS) {
    return { events: keyPair(CSI_KEYS[final]!, decodeXtermMods(modifierValue)), next };
  }
  if (final === '~') {
    const code = numbers[0];
    if (code !== undefined && TILDE_KEYS[code] !== undefined) {
      return { events: keyPair(TILDE_KEYS[code]!, decodeXtermMods(modifierValue)), next };
    }
    return { events: [], next };
  }
  return { events: [], next };
}

function parseEscape(
  buf: Uint8Array,
  start: number,
  limit: number,
  options: TerminalParseOptions,
  flush: boolean,
): { events: InputEvent[]; next: number } | null {
  if (start + 1 >= limit) {
    if (flush) return { events: keyPair(Key.Escape, 0), next: start + 1 };
    return null;
  }
  const nextByte = buf[start + 1]!;
  if (nextByte === 0x1b) {
    return { events: keyPair(Key.Escape, 0), next: start + 1 };
  }
  if (nextByte === 0x5b) {
    return parseCsi(buf, start + 1, limit, options, flush);
  }
  if (nextByte === 0x4f) {
    if (start + 2 >= limit) {
      if (flush) return { events: [], next: limit };
      return null;
    }
    const key = String.fromCharCode(buf[start + 2]!);
    const mapped = SS3_KEYS[key];
    return { events: mapped !== undefined ? keyPair(mapped, 0) : [], next: start + 3 };
  }
  if (nextByte === 0x5d) {
    let i = start + 2;
    while (i < limit) {
      const byte = buf[i]!;
      if (byte === 0x07) return { events: [], next: i + 1 };
      if (byte === 0x1b && i + 1 < limit && buf[i + 1] === 0x5c) return { events: [], next: i + 2 };
      i++;
    }
    if (flush) return { events: [], next: limit };
    return null;
  }
  if (nextByte === 0x50 || nextByte === 0x58 || nextByte === 0x5e || nextByte === 0x5f) {
    let i = start + 2;
    while (i < limit) {
      const byte = buf[i]!;
      if (byte === 0x07 || byte === 0x1b || (byte >= 0x40 && byte <= 0x7e)) {
        return { events: [], next: i + 1 };
      }
      i++;
    }
    if (flush) return { events: [], next: limit };
    return null;
  }
  if (nextByte < 0x20) {
    if (nextByte === 0x09) return { events: keyPair(Key.Tab, Modifier.Alt), next: start + 2 };
    if (nextByte === 0x0d || nextByte === 0x0a) return { events: keyPair(Key.Enter, Modifier.Alt), next: start + 2 };
    if (nextByte >= 1 && nextByte <= 26) {
      return { events: keyPair(Key.A + nextByte - 1, Modifier.Ctrl | Modifier.Alt), next: start + 2 };
    }
    return { events: [], next: start + 2 };
  }
  if (nextByte < 0x80) {
    const mapped = keyFromChar(String.fromCharCode(nextByte), Modifier.Alt);
    if (mapped) return { events: keyPair(mapped.key, mapped.modifiers), next: start + 2 };
    return { events: [], next: start + 2 };
  }
  const decoded = decodeUtf8(buf, start + 1, flush);
  if (!decoded) return null;
  const events: InputEvent[] = [];
  if (decoded.code >= 0x20) pushText(events, decoded.code);
  return { events, next: decoded.next };
}

function findSequence(buf: Uint8Array, from: number, limit: number, seq: number[]): number {
  outer: for (let i = from; i + seq.length <= limit; i++) {
    for (let j = 0; j < seq.length; j++) {
      if (buf[i + j] !== seq[j]) continue outer;
    }
    return i;
  }
  return -1;
}

export function parseTerminalInput(
  buf: Uint8Array,
  options: TerminalParseOptions,
  flush = false,
): TerminalParseResult {
  const limit = buf.length;
  const parsed = parsePlain(buf, 0, limit, options, flush);
  return { events: parsed.events, consumed: parsed.consumed };
}
