import koffi from 'koffi';
import { GhostwireError } from '../../../core/domain/errors.js';
import { Key, Modifier, type InputEvent } from '../../../core/domain/input/events.js';
import type { InputInjectorPort } from '../../../core/ports/outbound/inject.js';

const INPUT_MOUSE = 0;
const INPUT_KEYBOARD = 1;

const MOUSEEVENTF_MOVE = 0x0001;
const MOUSEEVENTF_LEFTDOWN = 0x0002;
const MOUSEEVENTF_LEFTUP = 0x0004;
const MOUSEEVENTF_RIGHTDOWN = 0x0008;
const MOUSEEVENTF_RIGHTUP = 0x0010;
const MOUSEEVENTF_MIDDLEDOWN = 0x0020;
const MOUSEEVENTF_MIDDLEUP = 0x0040;
const MOUSEEVENTF_XDOWN = 0x0080;
const MOUSEEVENTF_XUP = 0x0100;
const MOUSEEVENTF_WHEEL = 0x0800;
const MOUSEEVENTF_ABSOLUTE = 0x8000;

const KEYEVENTF_EXTENDEDKEY = 0x0001;
const KEYEVENTF_KEYUP = 0x0002;
const KEYEVENTF_UNICODE = 0x0004;

const XBUTTON1 = 1;
const XBUTTON2 = 2;

const EXTENDED_KEYS = new Set<number>([
  Key.Left,
  Key.Up,
  Key.Right,
  Key.Down,
  Key.Home,
  Key.End,
  Key.PageUp,
  Key.PageDown,
  Key.Insert,
  Key.Delete,
  Key.RCtrl,
  Key.RAlt,
  Key.LWin,
  Key.RWin,
  Key.Apps,
  Key.Divide,
  Key.PrintScreen,
]);

const MODIFIER_KEYS: Array<{ bit: number; vk: number }> = [
  { bit: Modifier.Shift, vk: Key.LShift },
  { bit: Modifier.Ctrl, vk: Key.LCtrl },
  { bit: Modifier.Alt, vk: Key.LAlt },
  { bit: Modifier.Meta, vk: Key.LWin },
];

interface InputApi {
  getSystemMetrics(index: number): number;
  sendInput(inputs: unknown[]): number;
  inputSize: number;
}

let cachedApi: InputApi | null = null;

function loadInputApi(): InputApi {
  if (cachedApi) return cachedApi;
  if (process.platform !== 'win32') {
    throw new GhostwireError('unsupported', 'input injection requires Windows (SendInput)');
  }

  const user32 = koffi.load('user32.dll');

  const MOUSEINPUT = koffi.struct('MOUSEINPUT', {
    dx: 'int32_t',
    dy: 'int32_t',
    mouseData: 'uint32_t',
    dwFlags: 'uint32_t',
    time: 'uint32_t',
    dwExtraInfo: 'uintptr_t',
  });
  const KEYBDINPUT = koffi.struct('KEYBDINPUT', {
    wVk: 'uint16_t',
    wScan: 'uint16_t',
    dwFlags: 'uint32_t',
    time: 'uint32_t',
    dwExtraInfo: 'uintptr_t',
  });
  const HARDWAREINPUT = koffi.struct('HARDWAREINPUT', {
    uMsg: 'uint32_t',
    wParamL: 'uint16_t',
    wParamH: 'uint16_t',
  });
  const INPUT = koffi.struct('INPUT', {
    type: 'uint32_t',
    u: koffi.union({
      mi: MOUSEINPUT,
      ki: KEYBDINPUT,
      hi: HARDWAREINPUT,
    }),
  });

  const GetSystemMetrics = user32.func('int __stdcall GetSystemMetrics(int index)');
  const SendInput = user32.func('unsigned int __stdcall SendInput(unsigned int cInputs, INPUT *pInputs, int cbSize)');

  cachedApi = {
    getSystemMetrics: (index) => GetSystemMetrics(index) as number,
    sendInput: (inputs) => SendInput(inputs.length, inputs, koffi.sizeof(INPUT)) as number,
    inputSize: koffi.sizeof(INPUT),
  };
  return cachedApi;
}

function isExtendedKey(vk: number): boolean {
  return EXTENDED_KEYS.has(vk);
}

export class SendInputInjector implements InputInjectorPort {
  private heldModifiers = 0;

  inject(event: InputEvent): void {
    const api = loadInputApi();
    const inputs = this.translate(api, event);
    if (inputs.length === 0) return;
    const sent = api.sendInput(inputs);
    if (sent !== inputs.length) {
      throw new GhostwireError(
        'internal',
        `SendInput delivered ${sent} of ${inputs.length} events (blocked by UIPI or another input consumer)`,
      );
    }
  }

  releaseAll(): void {
    if (!cachedApi || this.heldModifiers === 0) return;
    const api = cachedApi;
    const release = MODIFIER_KEYS.filter((mod) => (this.heldModifiers & mod.bit) !== 0).map((mod) =>
      this.keyInput(mod.vk, false),
    );
    this.heldModifiers = 0;
    if (release.length > 0) {
      api.sendInput(release);
    }
  }

  close(): void {
    this.releaseAll();
  }

  private translate(api: InputApi, event: InputEvent): unknown[] {
    switch (event.kind) {
      case 'key': {
        if (event.down) {
          return [...this.syncModifiers(event.modifiers), this.keyInput(event.key, true)];
        }
        const out = [this.keyInput(event.key, false), ...this.releaseModifiers()];
        return out;
      }
      case 'mouseMove':
        return [...this.syncModifiers(event.modifiers), this.mouseInput(api, MOUSEEVENTF_MOVE, 0, event.x, event.y)];
      case 'mouseButton':
        return [
          ...this.syncModifiers(event.modifiers),
          this.mouseInput(api, buttonFlags(event.button, event.down), buttonData(event.button), event.x, event.y),
        ];
      case 'wheel':
        return [
          ...this.syncModifiers(event.modifiers),
          this.mouseInput(api, MOUSEEVENTF_WHEEL, event.deltaY < 0 ? event.deltaY + 0x100000000 : event.deltaY, event.x, event.y),
        ];
      case 'text': {
        if (event.code < 0 || event.code > 0xffff) return [];
        return [
          this.textInput(event.code, true),
          this.textInput(event.code, false),
        ];
      }
      default:
        return [];
    }
  }

  private syncModifiers(target: number): unknown[] {
    const inputs: unknown[] = [];
    for (const mod of MODIFIER_KEYS) {
      if ((target & mod.bit) !== 0 && (this.heldModifiers & mod.bit) === 0) {
        inputs.push(this.keyInput(mod.vk, true));
        this.heldModifiers |= mod.bit;
      }
    }
    for (const mod of MODIFIER_KEYS) {
      if ((this.heldModifiers & mod.bit) !== 0 && (target & mod.bit) === 0) {
        inputs.push(this.keyInput(mod.vk, false));
        this.heldModifiers &= ~mod.bit;
      }
    }
    return inputs;
  }

  private releaseModifiers(): unknown[] {
    const inputs: unknown[] = [];
    for (const mod of MODIFIER_KEYS) {
      if ((this.heldModifiers & mod.bit) !== 0) {
        inputs.push(this.keyInput(mod.vk, false));
        this.heldModifiers &= ~mod.bit;
      }
    }
    return inputs;
  }

  private keyInput(vk: number, down: boolean): unknown {
    let flags = down ? 0 : KEYEVENTF_KEYUP;
    if (isExtendedKey(vk)) flags |= KEYEVENTF_EXTENDEDKEY;
    return {
      type: INPUT_KEYBOARD,
      u: {
        ki: {
          wVk: vk,
          wScan: 0,
          dwFlags: flags,
          time: 0,
          dwExtraInfo: 0,
        },
      },
    };
  }

  private textInput(code: number, down: boolean): unknown {
    return {
      type: INPUT_KEYBOARD,
      u: {
        ki: {
          wVk: 0,
          wScan: code,
          dwFlags: KEYEVENTF_UNICODE | (down ? 0 : KEYEVENTF_KEYUP),
          time: 0,
          dwExtraInfo: 0,
        },
      },
    };
  }

  private mouseInput(api: InputApi, flags: number, mouseData: number, x: number, y: number): unknown {
    const screenW = Math.max(1, api.getSystemMetrics(0));
    const screenH = Math.max(1, api.getSystemMetrics(1));
    const dx = Math.min(65535, Math.max(0, Math.round((clamp(x, 0, screenW - 1) * 65535) / (screenW - 1))));
    const dy = Math.min(65535, Math.max(0, Math.round((clamp(y, 0, screenH - 1) * 65535) / (screenH - 1))));
    return {
      type: INPUT_MOUSE,
      u: {
        mi: {
          dx,
          dy,
          mouseData,
          dwFlags: flags | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_MOVE,
          time: 0,
          dwExtraInfo: 0,
        },
      },
    };
  }
}

function buttonFlags(button: number, down: boolean): number {
  switch (button) {
    case 1:
      return down ? MOUSEEVENTF_RIGHTDOWN : MOUSEEVENTF_RIGHTUP;
    case 2:
      return down ? MOUSEEVENTF_MIDDLEDOWN : MOUSEEVENTF_MIDDLEUP;
    case 3:
    case 4:
      return down ? MOUSEEVENTF_XDOWN : MOUSEEVENTF_XUP;
    default:
      return down ? MOUSEEVENTF_LEFTDOWN : MOUSEEVENTF_LEFTUP;
  }
}

function buttonData(button: number): number {
  if (button === 4) return XBUTTON2;
  if (button === 3) return XBUTTON1;
  return 0;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
