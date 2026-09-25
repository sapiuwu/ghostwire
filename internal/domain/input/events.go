package input

import (
	"encoding/binary"
	"fmt"

	"ghostwire/internal/domain"
)

const (
	InputKindKey         uint8 = 1
	InputKindMouseMove   uint8 = 2
	InputKindMouseButton uint8 = 3
	InputKindWheel       uint8 = 4
	InputKindText        uint8 = 5
)

const (
	InputFlagDown uint8 = 1 << 0
)

type Modifier uint16

const (
	ModNone  Modifier = 0
	ModShift Modifier = 1 << 0
	ModCtrl  Modifier = 1 << 1
	ModAlt   Modifier = 1 << 2
	ModMeta  Modifier = 1 << 3
)

type MouseButton uint16

const (
	MouseLeft   MouseButton = 0
	MouseRight  MouseButton = 1
	MouseMiddle MouseButton = 2
	MouseX1     MouseButton = 3
	MouseX2     MouseButton = 4
)

type Key uint16

const (
	KeyBackspace   Key = 0x08
	KeyTab         Key = 0x09
	KeyEnter       Key = 0x0d
	KeyPause       Key = 0x13
	KeyCapsLock    Key = 0x14
	KeyEscape      Key = 0x1b
	KeySpace       Key = 0x20
	KeyPageUp      Key = 0x21
	KeyPageDown    Key = 0x22
	KeyEnd         Key = 0x23
	KeyHome        Key = 0x24
	KeyLeft        Key = 0x25
	KeyUp          Key = 0x26
	KeyRight       Key = 0x27
	KeyDown        Key = 0x28
	KeyPrintScreen Key = 0x2c
	KeyInsert      Key = 0x2d
	KeyDelete      Key = 0x2e
	KeyDigit0      Key = 0x30
	KeyDigit1      Key = 0x31
	KeyDigit2      Key = 0x32
	KeyDigit3      Key = 0x33
	KeyDigit4      Key = 0x34
	KeyDigit5      Key = 0x35
	KeyDigit6      Key = 0x36
	KeyDigit7      Key = 0x37
	KeyDigit8      Key = 0x38
	KeyDigit9      Key = 0x39
	KeyA           Key = 0x41
	KeyB           Key = 0x42
	KeyC           Key = 0x43
	KeyD           Key = 0x44
	KeyE           Key = 0x45
	KeyF           Key = 0x46
	KeyG           Key = 0x47
	KeyH           Key = 0x48
	KeyI           Key = 0x49
	KeyJ           Key = 0x4a
	KeyK           Key = 0x4b
	KeyL           Key = 0x4c
	KeyM           Key = 0x4d
	KeyN           Key = 0x4e
	KeyO           Key = 0x4f
	KeyP           Key = 0x50
	KeyQ           Key = 0x51
	KeyR           Key = 0x52
	KeyS           Key = 0x53
	KeyT           Key = 0x54
	KeyU           Key = 0x55
	KeyV           Key = 0x56
	KeyW           Key = 0x57
	KeyX           Key = 0x58
	KeyY           Key = 0x59
	KeyZ           Key = 0x5a
	KeyLWin        Key = 0x5b
	KeyRWin        Key = 0x5c
	KeyApps        Key = 0x5d
	KeyNumpad0     Key = 0x60
	KeyNumpad1     Key = 0x61
	KeyNumpad2     Key = 0x62
	KeyNumpad3     Key = 0x63
	KeyNumpad4     Key = 0x64
	KeyNumpad5     Key = 0x65
	KeyNumpad6     Key = 0x66
	KeyNumpad7     Key = 0x67
	KeyNumpad8     Key = 0x68
	KeyNumpad9     Key = 0x69
	KeyMultiply    Key = 0x6a
	KeyAdd         Key = 0x6b
	KeySubtract    Key = 0x6d
	KeyDecimal     Key = 0x6e
	KeyDivide      Key = 0x6f
	KeyF1          Key = 0x70
	KeyF2          Key = 0x71
	KeyF3          Key = 0x72
	KeyF4          Key = 0x73
	KeyF5          Key = 0x74
	KeyF6          Key = 0x75
	KeyF7          Key = 0x76
	KeyF8          Key = 0x77
	KeyF9          Key = 0x78
	KeyF10         Key = 0x79
	KeyF11         Key = 0x7a
	KeyF12         Key = 0x7b
	KeyNumLock     Key = 0x90
	KeyScrollLock  Key = 0x91
	KeyLShift      Key = 0xa0
	KeyRShift      Key = 0xa1
	KeyLCtrl       Key = 0xa2
	KeyRCtrl       Key = 0xa3
	KeyLAlt        Key = 0xa4
	KeyRAlt        Key = 0xa5
	KeyOem1        Key = 0xba
	KeyOemPlus     Key = 0xbb
	KeyOemComma    Key = 0xbc
	KeyOemMinus    Key = 0xbd
	KeyOemPeriod   Key = 0xbe
	KeyOem2        Key = 0xbf
	KeyOem3        Key = 0xc0
	KeyOem4        Key = 0xdb
	KeyOem5        Key = 0xdc
	KeyOem6        Key = 0xdd
	KeyOem7        Key = 0xde
)

type InputEvent interface {
	inputEvent()
}

type KeyEvent struct {
	Key       Key
	Down      bool
	Modifiers Modifier
}

func (KeyEvent) inputEvent() {}

type MouseMoveEvent struct {
	X         int32
	Y         int32
	Modifiers Modifier
}

func (MouseMoveEvent) inputEvent() {}

type MouseButtonEvent struct {
	Button    MouseButton
	Down      bool
	X         int32
	Y         int32
	Modifiers Modifier
}

func (MouseButtonEvent) inputEvent() {}

type WheelEvent struct {
	X         int32
	Y         int32
	DeltaY    int16
	Modifiers Modifier
}

func (WheelEvent) inputEvent() {}

type TextEvent struct {
	Code uint16
}

func (TextEvent) inputEvent() {}

func EncodeInputEvent(ev InputEvent) []byte {
	buf := make([]byte, 20)
	switch e := ev.(type) {
	case KeyEvent:
		buf[0] = InputKindKey
		if e.Down {
			buf[1] = InputFlagDown
		}
		binary.LittleEndian.PutUint16(buf[2:4], uint16(e.Modifiers))
		binary.LittleEndian.PutUint16(buf[4:6], uint16(e.Key))
	case MouseMoveEvent:
		buf[0] = InputKindMouseMove
		binary.LittleEndian.PutUint16(buf[2:4], uint16(e.Modifiers))
		binary.LittleEndian.PutUint32(buf[8:12], uint32(e.X))
		binary.LittleEndian.PutUint32(buf[12:16], uint32(e.Y))
	case MouseButtonEvent:
		buf[0] = InputKindMouseButton
		if e.Down {
			buf[1] = InputFlagDown
		}
		binary.LittleEndian.PutUint16(buf[2:4], uint16(e.Modifiers))
		binary.LittleEndian.PutUint16(buf[4:6], uint16(e.Button))
		binary.LittleEndian.PutUint32(buf[8:12], uint32(e.X))
		binary.LittleEndian.PutUint32(buf[12:16], uint32(e.Y))
	case WheelEvent:
		buf[0] = InputKindWheel
		binary.LittleEndian.PutUint16(buf[2:4], uint16(e.Modifiers))
		binary.LittleEndian.PutUint32(buf[8:12], uint32(e.X))
		binary.LittleEndian.PutUint32(buf[12:16], uint32(e.Y))
		binary.LittleEndian.PutUint16(buf[16:18], uint16(e.DeltaY))
	case TextEvent:
		buf[0] = InputKindText
		binary.LittleEndian.PutUint16(buf[4:6], e.Code)
	}
	return buf
}

func DecodeInputEvent(data []byte) (InputEvent, error) {
	if len(data) != 20 {
		return nil, domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("input event must be 20 bytes, got %d", len(data)))
	}
	kind := data[0]
	flags := data[1]
	modifiers := Modifier(binary.LittleEndian.Uint16(data[2:4]))
	code := binary.LittleEndian.Uint16(data[4:6])
	x := int32(binary.LittleEndian.Uint32(data[8:12]))
	y := int32(binary.LittleEndian.Uint32(data[12:16]))
	deltaY := int16(binary.LittleEndian.Uint16(data[16:18]))
	down := flags&InputFlagDown != 0

	switch kind {
	case InputKindKey:
		return KeyEvent{Key: Key(code), Down: down, Modifiers: modifiers}, nil
	case InputKindMouseMove:
		return MouseMoveEvent{X: x, Y: y, Modifiers: modifiers}, nil
	case InputKindMouseButton:
		return MouseButtonEvent{Button: MouseButton(code), Down: down, X: x, Y: y, Modifiers: modifiers}, nil
	case InputKindWheel:
		return WheelEvent{X: x, Y: y, DeltaY: deltaY, Modifiers: modifiers}, nil
	case InputKindText:
		return TextEvent{Code: code}, nil
	default:
		return nil, domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("unknown input kind %d", kind))
	}
}
