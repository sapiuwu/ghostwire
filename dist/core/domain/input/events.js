import { GhostwireError } from '../errors.js';
export const Modifier = {
    None: 0,
    Shift: 1 << 0,
    Ctrl: 1 << 1,
    Alt: 1 << 2,
    Meta: 1 << 3,
};
export const MouseButton = {
    Left: 0,
    Right: 1,
    Middle: 2,
    X1: 3,
    X2: 4,
};
export const Key = {
    Backspace: 0x08,
    Tab: 0x09,
    Enter: 0x0d,
    Pause: 0x13,
    CapsLock: 0x14,
    Escape: 0x1b,
    Space: 0x20,
    PageUp: 0x21,
    PageDown: 0x22,
    End: 0x23,
    Home: 0x24,
    Left: 0x25,
    Up: 0x26,
    Right: 0x27,
    Down: 0x28,
    PrintScreen: 0x2c,
    Insert: 0x2d,
    Delete: 0x2e,
    Digit0: 0x30,
    Digit1: 0x31,
    Digit2: 0x32,
    Digit3: 0x33,
    Digit4: 0x34,
    Digit5: 0x35,
    Digit6: 0x36,
    Digit7: 0x37,
    Digit8: 0x38,
    Digit9: 0x39,
    A: 0x41,
    B: 0x42,
    C: 0x43,
    D: 0x44,
    E: 0x45,
    F: 0x46,
    G: 0x47,
    H: 0x48,
    I: 0x49,
    J: 0x4a,
    K: 0x4b,
    L: 0x4c,
    M: 0x4d,
    N: 0x4e,
    O: 0x4f,
    P: 0x50,
    Q: 0x51,
    R: 0x52,
    S: 0x53,
    T: 0x54,
    U: 0x55,
    V: 0x56,
    W: 0x57,
    X: 0x58,
    Y: 0x59,
    Z: 0x5a,
    LWin: 0x5b,
    RWin: 0x5c,
    Apps: 0x5d,
    NumPad0: 0x60,
    NumPad1: 0x61,
    NumPad2: 0x62,
    NumPad3: 0x63,
    NumPad4: 0x64,
    NumPad5: 0x65,
    NumPad6: 0x66,
    NumPad7: 0x67,
    NumPad8: 0x68,
    NumPad9: 0x69,
    Multiply: 0x6a,
    Add: 0x6b,
    Subtract: 0x6d,
    Decimal: 0x6e,
    Divide: 0x6f,
    F1: 0x70,
    F2: 0x71,
    F3: 0x72,
    F4: 0x73,
    F5: 0x74,
    F6: 0x75,
    F7: 0x76,
    F8: 0x77,
    F9: 0x78,
    F10: 0x79,
    F11: 0x7a,
    F12: 0x7b,
    NumLock: 0x90,
    ScrollLock: 0x91,
    LShift: 0xa0,
    RShift: 0xa1,
    LCtrl: 0xa2,
    RCtrl: 0xa3,
    LAlt: 0xa4,
    RAlt: 0xa5,
    Oem1: 0xba,
    OemPlus: 0xbb,
    OemComma: 0xbc,
    OemMinus: 0xbd,
    OemPeriod: 0xbe,
    Oem2: 0xbf,
    Oem3: 0xc0,
    Oem4: 0xdb,
    Oem5: 0xdc,
    Oem6: 0xdd,
    Oem7: 0xde,
};
export const InputKind = {
    Key: 1,
    MouseMove: 2,
    MouseButton: 3,
    Wheel: 4,
    Text: 5,
};
const INPUT_FLAG_DOWN = 1 << 0;
export function encodeInputEvent(ev) {
    const buf = new Uint8Array(20);
    const view = new DataView(buf.buffer);
    switch (ev.kind) {
        case 'key':
            view.setUint8(0, InputKind.Key);
            view.setUint8(1, ev.down ? INPUT_FLAG_DOWN : 0);
            view.setUint16(2, ev.modifiers);
            view.setUint16(4, ev.key);
            break;
        case 'mouseMove':
            view.setUint8(0, InputKind.MouseMove);
            view.setUint16(2, ev.modifiers);
            view.setInt32(8, ev.x);
            view.setInt32(12, ev.y);
            break;
        case 'mouseButton':
            view.setUint8(0, InputKind.MouseButton);
            view.setUint8(1, ev.down ? INPUT_FLAG_DOWN : 0);
            view.setUint16(2, ev.modifiers);
            view.setUint16(4, ev.button);
            view.setInt32(8, ev.x);
            view.setInt32(12, ev.y);
            break;
        case 'wheel':
            view.setUint8(0, InputKind.Wheel);
            view.setUint16(2, ev.modifiers);
            view.setInt32(8, ev.x);
            view.setInt32(12, ev.y);
            view.setInt16(16, ev.deltaY);
            break;
        case 'text':
            view.setUint8(0, InputKind.Text);
            view.setUint16(4, ev.code);
            break;
    }
    return buf;
}
export function decodeInputEvent(data) {
    if (data.length !== 20) {
        throw new GhostwireError('protocol', `input event must be 20 bytes, got ${data.length}`);
    }
    const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
    const kind = view.getUint8(0);
    const flags = view.getUint8(1);
    const modifiers = view.getUint16(2);
    const code = view.getUint16(4);
    const x = view.getInt32(8);
    const y = view.getInt32(12);
    const deltaY = view.getInt16(16);
    const down = (flags & INPUT_FLAG_DOWN) !== 0;
    switch (kind) {
        case InputKind.Key:
            return { kind: 'key', key: code, down, modifiers };
        case InputKind.MouseMove:
            return { kind: 'mouseMove', x, y, modifiers };
        case InputKind.MouseButton:
            return { kind: 'mouseButton', button: code, down, x, y, modifiers };
        case InputKind.Wheel:
            return { kind: 'wheel', x, y, deltaY, modifiers };
        case InputKind.Text:
            return { kind: 'text', code };
        default:
            throw new GhostwireError('protocol', `unknown input kind ${kind}`);
    }
}
//# sourceMappingURL=events.js.map