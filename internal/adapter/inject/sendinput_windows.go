//go:build windows

package inject

import (
	"fmt"
	"syscall"
	"unsafe"

	"ghostwire/internal/domain"
	"ghostwire/internal/domain/input"
	"ghostwire/internal/port"
)

var (
	moduser32 = syscall.NewLazyDLL("user32.dll")

	procGetSystemMetrics = moduser32.NewProc("GetSystemMetrics")
	procSendInput        = moduser32.NewProc("SendInput")
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseEventFMove       = 0x0001
	mouseEventFLeftDown   = 0x0002
	mouseEventFLeftUp     = 0x0004
	mouseEventFRightDown  = 0x0008
	mouseEventFRightUp    = 0x0010
	mouseEventFMiddleDown = 0x0020
	mouseEventFMiddleUp   = 0x0040
	mouseEventFXDown      = 0x0080
	mouseEventFXUp        = 0x0100
	mouseEventFWheel      = 0x0800
	mouseEventFAbsolute   = 0x8000

	keyEventFExtendedKey = 0x0001
	keyEventFKeyUp       = 0x0002
	keyEventFUnicode     = 0x0004

	xbutton1 = 1
	xbutton2 = 2
)

type mouseInput struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type keybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type hardwareInput struct {
	UMsg    uint32
	WParamL uint16
	WParamH uint16
}

type winInput struct {
	Type uint32
	Mi   mouseInput
	Ki   keybdInput
	Hi   hardwareInput
}

var extendedKeys = map[input.Key]bool{
	input.KeyLeft: true, input.KeyUp: true, input.KeyRight: true, input.KeyDown: true,
	input.KeyHome: true, input.KeyEnd: true, input.KeyPageUp: true, input.KeyPageDown: true,
	input.KeyInsert: true, input.KeyDelete: true,
	input.KeyRCtrl: true, input.KeyRAlt: true, input.KeyLWin: true, input.KeyRWin: true,
	input.KeyApps: true, input.KeyDivide: true, input.KeyPrintScreen: true,
}

var modifierKeys = []struct {
	bit input.Modifier
	vk  input.Key
}{
	{bit: input.ModShift, vk: input.KeyLShift},
	{bit: input.ModCtrl, vk: input.KeyLCtrl},
	{bit: input.ModAlt, vk: input.KeyLAlt},
	{bit: input.ModMeta, vk: input.KeyLWin},
}

type SendInputInjector struct {
	heldModifiers input.Modifier
}

func NewSendInputInjector() *SendInputInjector {
	return &SendInputInjector{}
}

func getSystemMetrics(index int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(r)
}

func doSendInput(inputs []winInput) uint32 {
	if len(inputs) == 0 {
		return 0
	}
	r, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(winInput{}),
	)
	return uint32(r)
}

func (inj *SendInputInjector) Inject(ev input.InputEvent) {
	inputs := inj.translate(ev)
	if len(inputs) == 0 {
		return
	}
	sent := doSendInput(inputs)
	if sent != uint32(len(inputs)) {
		panic(domain.NewError(domain.ErrInternal,
			fmt.Sprintf("SendInput delivered %d of %d events", sent, len(inputs))))
	}
}

func (inj *SendInputInjector) ReleaseAll() {
	if inj.heldModifiers == 0 {
		return
	}
	var releases []winInput
	for _, mod := range modifierKeys {
		if inj.heldModifiers&mod.bit != 0 {
			releases = append(releases, inj.keyInput(mod.vk, false))
		}
	}
	inj.heldModifiers = 0
	if len(releases) > 0 {
		doSendInput(releases)
	}
}

func (inj *SendInputInjector) Close() {
	inj.ReleaseAll()
}

func (inj *SendInputInjector) translate(ev input.InputEvent) []winInput {
	switch e := ev.(type) {
	case input.KeyEvent:
		if e.Down {
			out := inj.syncModifiers(e.Modifiers)
			out = append(out, inj.keyInput(e.Key, true))
			return out
		}
		out := []winInput{inj.keyInput(e.Key, false)}
		out = append(out, inj.releaseModifiers()...)
		return out
	case input.MouseMoveEvent:
		out := inj.syncModifiers(e.Modifiers)
		out = append(out, inj.mouseInput(mouseEventFMove, 0, e.X, e.Y))
		return out
	case input.MouseButtonEvent:
		out := inj.syncModifiers(e.Modifiers)
		out = append(out, inj.mouseInput(buttonFlags(e.Button, e.Down), buttonData(e.Button), e.X, e.Y))
		return out
	case input.WheelEvent:
		out := inj.syncModifiers(e.Modifiers)
		var md uint32
		if e.DeltaY >= 0 {
			md = uint32(e.DeltaY)
		} else {
			md = uint32(int32(e.DeltaY))
		}
		out = append(out, inj.mouseInput(mouseEventFWheel, md, e.X, e.Y))
		return out
	case input.TextEvent:
		if e.Code > 0xffff {
			return nil
		}
		return []winInput{
			inj.textInput(e.Code, true),
			inj.textInput(e.Code, false),
		}
	}
	return nil
}

func (inj *SendInputInjector) syncModifiers(target input.Modifier) []winInput {
	var inputs []winInput
	for _, mod := range modifierKeys {
		if target&mod.bit != 0 && inj.heldModifiers&mod.bit == 0 {
			inputs = append(inputs, inj.keyInput(mod.vk, true))
			inj.heldModifiers |= mod.bit
		}
	}
	for _, mod := range modifierKeys {
		if inj.heldModifiers&mod.bit != 0 && target&mod.bit == 0 {
			inputs = append(inputs, inj.keyInput(mod.vk, false))
			inj.heldModifiers &^= mod.bit
		}
	}
	return inputs
}

func (inj *SendInputInjector) releaseModifiers() []winInput {
	var inputs []winInput
	for _, mod := range modifierKeys {
		if inj.heldModifiers&mod.bit != 0 {
			inputs = append(inputs, inj.keyInput(mod.vk, false))
			inj.heldModifiers &^= mod.bit
		}
	}
	return inputs
}

func (inj *SendInputInjector) keyInput(key input.Key, down bool) winInput {
	flags := uint32(0)
	if !down {
		flags = keyEventFKeyUp
	}
	if extendedKeys[key] {
		flags |= keyEventFExtendedKey
	}
	return winInput{
		Type: inputKeyboard,
		Ki: keybdInput{
			WVk:     uint16(key),
			DwFlags: flags,
		},
	}
}

func (inj *SendInputInjector) textInput(code uint16, down bool) winInput {
	flags := uint32(keyEventFUnicode)
	if !down {
		flags |= keyEventFKeyUp
	}
	return winInput{
		Type: inputKeyboard,
		Ki: keybdInput{
			WScan:   code,
			DwFlags: flags,
		},
	}
}

func (inj *SendInputInjector) mouseInput(flags uint32, mouseData uint32, x, y int32) winInput {
	screenW := getSystemMetrics(0)
	screenH := getSystemMetrics(1)
	if screenW <= 1 {
		screenW = 1
	}
	if screenH <= 1 {
		screenH = 1
	}
	dx := clamp(float64(x), 0, float64(screenW-1))
	dy := clamp(float64(y), 0, float64(screenH-1))
	dxi := uint32(dx * 65535 / float64(screenW-1))
	dyi := uint32(dy * 65535 / float64(screenH-1))
	return winInput{
		Type: inputMouse,
		Mi: mouseInput{
			Dx:        int32(dxi),
			Dy:        int32(dyi),
			MouseData: mouseData,
			DwFlags:   flags | mouseEventFAbsolute | mouseEventFMove,
		},
	}
}

func buttonFlags(button input.MouseButton, down bool) uint32 {
	switch button {
	case input.MouseRight:
		if down {
			return mouseEventFRightDown
		}
		return mouseEventFRightUp
	case input.MouseMiddle:
		if down {
			return mouseEventFMiddleDown
		}
		return mouseEventFMiddleUp
	case input.MouseX1, input.MouseX2:
		if down {
			return mouseEventFXDown
		}
		return mouseEventFXUp
	default:
		if down {
			return mouseEventFLeftDown
		}
		return mouseEventFLeftUp
	}
}

func buttonData(button input.MouseButton) uint32 {
	if button == input.MouseX2 {
		return xbutton2
	}
	if button == input.MouseX1 {
		return xbutton1
	}
	return 0
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

var _ port.InputInjectorPort = (*SendInputInjector)(nil)
