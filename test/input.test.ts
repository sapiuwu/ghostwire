import { describe, expect, it } from 'vitest';
import { GhostwireError } from '../src/core/domain/errors.js';
import {
  Key,
  Modifier,
  decodeInputEvent,
  encodeInputEvent,
  type InputEvent,
} from '../src/core/domain/input/events.js';
import { keyFromChar } from '../src/adapters/primary/terminal/input-parser.js';

describe('input event wire codec', () => {
  const cases: InputEvent[] = [
    { kind: 'key', key: Key.A, down: true, modifiers: 0 },
    { kind: 'key', key: Key.F12, down: false, modifiers: Modifier.Ctrl | Modifier.Shift },
    { kind: 'mouseMove', x: 1919, y: 1079, modifiers: Modifier.Alt },
    { kind: 'mouseButton', button: 1, down: true, x: 10, y: 20, modifiers: 0 },
    { kind: 'mouseButton', button: 2, down: false, x: 0, y: 0, modifiers: Modifier.Meta },
    { kind: 'wheel', x: 100, y: 200, deltaY: -120, modifiers: 0 },
    { kind: 'wheel', x: 1, y: 2, deltaY: 120, modifiers: Modifier.Ctrl },
    { kind: 'text', code: 0x4e2d },
    { kind: 'text', code: 0xd83d },
  ];

  for (const event of cases) {
    it(`roundtrips ${event.kind} ${JSON.stringify(event)}`, () => {
      expect(decodeInputEvent(encodeInputEvent(event))).toEqual(event);
    });
  }

  it('rejects wrong length', () => {
    expect(() => decodeInputEvent(new Uint8Array(19))).toThrowError(/20 bytes/);
  });

  it('rejects unknown kind', () => {
    const data = encodeInputEvent({ kind: 'key', key: 1, down: true, modifiers: 0 });
    data[0] = 99;
    expect(() => decodeInputEvent(data)).toThrowError(/unknown input kind/);
  });
});

describe('keyFromChar', () => {
  it('maps plain letters', () => {
    expect(keyFromChar('a')).toEqual({ key: Key.A, modifiers: 0 });
    expect(keyFromChar('z')).toEqual({ key: Key.Z, modifiers: 0 });
  });

  it('maps uppercase letters to shift', () => {
    expect(keyFromChar('A')).toEqual({ key: Key.A, modifiers: Modifier.Shift });
  });

  it('maps digits without shift', () => {
    expect(keyFromChar('0')).toEqual({ key: Key.Digit0, modifiers: 0 });
    expect(keyFromChar('9')).toEqual({ key: Key.Digit9, modifiers: 0 });
  });

  it('maps shifted symbols to their base keys', () => {
    expect(keyFromChar('!')).toEqual({ key: Key.Digit1, modifiers: Modifier.Shift });
    expect(keyFromChar('@')).toEqual({ key: Key.Digit2, modifiers: Modifier.Shift });
    expect(keyFromChar('(')).toEqual({ key: Key.Digit9, modifiers: Modifier.Shift });
    expect(keyFromChar('+')).toEqual({ key: Key.OemPlus, modifiers: Modifier.Shift });
    expect(keyFromChar('"')).toEqual({ key: Key.Oem7, modifiers: Modifier.Shift });
  });

  it('maps plain punctuation', () => {
    expect(keyFromChar('-')).toEqual({ key: Key.OemMinus, modifiers: 0 });
    expect(keyFromChar('/')).toEqual({ key: Key.Oem2, modifiers: 0 });
    expect(keyFromChar('`')).toEqual({ key: Key.Oem3, modifiers: 0 });
    expect(keyFromChar(' ')).toEqual({ key: Key.Space, modifiers: 0 });
  });

  it('applies base modifiers', () => {
    expect(keyFromChar('a', Modifier.Alt)).toEqual({ key: Key.A, modifiers: Modifier.Alt });
    expect(keyFromChar('A', Modifier.Alt)).toEqual({
      key: Key.A,
      modifiers: Modifier.Alt | Modifier.Shift,
    });
  });

  it('returns null for unmapped characters', () => {
    expect(keyFromChar('é')).toBeNull();
    expect(keyFromChar('中')).toBeNull();
    expect(keyFromChar('')).toBeNull();
  });
});
