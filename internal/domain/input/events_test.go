package input_test

import (
	"bytes"
	"testing"

	"ghostwire/internal/domain/input"
)

func TestRoundtripInputEvents(t *testing.T) {
	cases := []input.InputEvent{
		input.KeyEvent{Key: input.KeyA, Down: true, Modifiers: 0},
		input.KeyEvent{Key: input.KeyF12, Down: false, Modifiers: input.ModCtrl | input.ModShift},
		input.MouseMoveEvent{X: 1919, Y: 1079, Modifiers: input.ModAlt},
		input.MouseButtonEvent{Button: input.MouseRight, Down: true, X: 10, Y: 20, Modifiers: 0},
		input.MouseButtonEvent{Button: input.MouseMiddle, Down: false, X: 0, Y: 0, Modifiers: input.ModMeta},
		input.WheelEvent{X: 100, Y: 200, DeltaY: -120, Modifiers: 0},
		input.WheelEvent{X: 1, Y: 2, DeltaY: 120, Modifiers: input.ModCtrl},
		input.TextEvent{Code: 0x4e2d},
		input.TextEvent{Code: 0xd83d},
	}

	for _, ev := range cases {
		encoded := input.EncodeInputEvent(ev)
		decoded, err := input.DecodeInputEvent(encoded)
		if err != nil {
			t.Fatalf("DecodeInputEvent failed for %T: %v", ev, err)
		}
		if !eventsEqual(ev, decoded) {
			t.Errorf("roundtrip mismatch: input %v, got %v", ev, decoded)
		}
	}
}

func TestRejectWrongLength(t *testing.T) {
	_, err := input.DecodeInputEvent(make([]byte, 19))
	if err == nil {
		t.Fatal("expected error for wrong length")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("20 bytes")) {
		t.Errorf("expected '20 bytes' error, got: %v", err)
	}
}

func TestRejectUnknownKind(t *testing.T) {
	data := input.EncodeInputEvent(input.KeyEvent{Key: 1, Down: true, Modifiers: 0})
	data[0] = 99
	_, err := input.DecodeInputEvent(data)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("unknown input kind")) {
		t.Errorf("expected 'unknown input kind' error, got: %v", err)
	}
}

func TestEncodeInputEventSize(t *testing.T) {
	ev := input.EncodeInputEvent(input.KeyEvent{Key: input.KeyA, Down: true, Modifiers: 0})
	if len(ev) != 20 {
		t.Errorf("expected 20 bytes, got %d", len(ev))
	}
}

func TestKeyEventFlags(t *testing.T) {
	down := input.EncodeInputEvent(input.KeyEvent{Key: input.KeyA, Down: true, Modifiers: 0})
	if down[1] != 1 {
		t.Errorf("expected flag bit0 set for key down, got %d", down[1])
	}

	up := input.EncodeInputEvent(input.KeyEvent{Key: input.KeyA, Down: false, Modifiers: 0})
	if up[1] != 0 {
		t.Errorf("expected flag bit0 clear for key up, got %d", up[1])
	}
}

func TestMouseMoveModifiers(t *testing.T) {
	ev := input.EncodeInputEvent(input.MouseMoveEvent{X: 100, Y: 200, Modifiers: input.ModShift | input.ModCtrl})
	mod := uint16(ev[2]) | uint16(ev[3])<<8
	if mod&uint16(input.ModShift) == 0 {
		t.Error("expected Shift modifier set")
	}
	if mod&uint16(input.ModCtrl) == 0 {
		t.Error("expected Ctrl modifier set")
	}
}

func TestMouseButtonCoords(t *testing.T) {
	ev := input.EncodeInputEvent(input.MouseButtonEvent{
		Button: input.MouseLeft, Down: true, X: 500, Y: 300, Modifiers: 0,
	})
	x := int32(uint32(ev[8]) | uint32(ev[9])<<8 | uint32(ev[10])<<16 | uint32(ev[11])<<24)
	y := int32(uint32(ev[12]) | uint32(ev[13])<<8 | uint32(ev[14])<<16 | uint32(ev[15])<<24)
	if x != 500 {
		t.Errorf("expected x=500, got %d", x)
	}
	if y != 300 {
		t.Errorf("expected y=300, got %d", y)
	}
}

func TestWheelDeltaY(t *testing.T) {
	ev := input.EncodeInputEvent(input.WheelEvent{X: 0, Y: 0, DeltaY: -120, Modifiers: 0})
	dy := int16(uint16(ev[16]) | uint16(ev[17])<<8)
	if dy != -120 {
		t.Errorf("expected deltaY=-120, got %d", dy)
	}
}

func TestTextEventCode(t *testing.T) {
	ev := input.EncodeInputEvent(input.TextEvent{Code: 0x4e2d})
	code := uint16(ev[4]) | uint16(ev[5])<<8
	if code != 0x4e2d {
		t.Errorf("expected code 0x4e2d, got 0x%04x", code)
	}
}

func eventsEqual(a, b input.InputEvent) bool {
	switch ea := a.(type) {
	case input.KeyEvent:
		eb, ok := b.(input.KeyEvent)
		if !ok {
			return false
		}
		return ea.Key == eb.Key && ea.Down == eb.Down && ea.Modifiers == eb.Modifiers
	case input.MouseMoveEvent:
		eb, ok := b.(input.MouseMoveEvent)
		if !ok {
			return false
		}
		return ea.X == eb.X && ea.Y == eb.Y && ea.Modifiers == eb.Modifiers
	case input.MouseButtonEvent:
		eb, ok := b.(input.MouseButtonEvent)
		if !ok {
			return false
		}
		return ea.Button == eb.Button && ea.Down == eb.Down && ea.X == eb.X && ea.Y == eb.Y && ea.Modifiers == eb.Modifiers
	case input.WheelEvent:
		eb, ok := b.(input.WheelEvent)
		if !ok {
			return false
		}
		return ea.X == eb.X && ea.Y == eb.Y && ea.DeltaY == eb.DeltaY && ea.Modifiers == eb.Modifiers
	case input.TextEvent:
		eb, ok := b.(input.TextEvent)
		if !ok {
			return false
		}
		return ea.Code == eb.Code
	}
	return false
}
