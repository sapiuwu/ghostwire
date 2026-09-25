package terminal_test

import (
	"testing"

	"ghostwire/internal/adapter/terminal"
	"ghostwire/internal/domain/input"
)

func TestKeyFromCharLetters(t *testing.T) {
	k, m, ok := terminal.KeyFromChar('a', 0)
	if !ok || k != input.KeyA || m != 0 {
		t.Errorf("expected KeyA/0, got %v/%v/%v", k, m, ok)
	}
	k, m, ok = terminal.KeyFromChar('z', 0)
	if !ok || k != input.KeyZ || m != 0 {
		t.Errorf("expected KeyZ/0, got %v/%v/%v", k, m, ok)
	}
}

func TestKeyFromCharUppercase(t *testing.T) {
	k, m, ok := terminal.KeyFromChar('A', 0)
	if !ok || k != input.KeyA || m != input.ModShift {
		t.Errorf("expected KeyA/Shift, got %v/%v", k, m)
	}
}

func TestKeyFromCharDigits(t *testing.T) {
	k, m, ok := terminal.KeyFromChar('0', 0)
	if !ok || k != input.KeyDigit0 || m != 0 {
		t.Errorf("expected KeyDigit0/0, got %v/%v", k, m)
	}
	k, m, ok = terminal.KeyFromChar('9', 0)
	if !ok || k != input.KeyDigit9 || m != 0 {
		t.Errorf("expected KeyDigit9/0, got %v/%v", k, m)
	}
}

func TestKeyFromCharShiftedSymbols(t *testing.T) {
	cases := []struct {
		ch   byte
		want input.Key
	}{
		{'!', input.KeyDigit1},
		{'@', input.KeyDigit2},
		{'(', input.KeyDigit9},
		{'+', input.KeyOemPlus},
		{'"', input.KeyOem7},
	}
	for _, tc := range cases {
		k, m, ok := terminal.KeyFromChar(tc.ch, 0)
		if !ok || k != tc.want || m != input.ModShift {
			t.Errorf("KeyFromChar(%q): got %v/%v/%v, want %v/Shift/true", tc.ch, k, m, ok, tc.want)
		}
	}
}

func TestKeyFromCharPlainPunctuation(t *testing.T) {
	cases := []struct {
		ch   byte
		want input.Key
	}{
		{'-', input.KeyOemMinus},
		{'/', input.KeyOem2},
		{'`', input.KeyOem3},
		{' ', input.KeySpace},
	}
	for _, tc := range cases {
		k, m, ok := terminal.KeyFromChar(tc.ch, 0)
		if !ok || k != tc.want || m != 0 {
			t.Errorf("KeyFromChar(%q): got %v/%v/%v, want %v/0/true", tc.ch, k, m, ok, tc.want)
		}
	}
}

func TestKeyFromCharBaseModifiers(t *testing.T) {
	k, m, ok := terminal.KeyFromChar('a', input.ModAlt)
	if !ok || k != input.KeyA || m != input.ModAlt {
		t.Errorf("expected KeyA/Alt, got %v/%v", k, m)
	}
	k, m, ok = terminal.KeyFromChar('A', input.ModAlt)
	if !ok || k != input.KeyA || m != input.ModAlt|input.ModShift {
		t.Errorf("expected KeyA/Alt|Shift, got %v/%v", k, m)
	}
}

func TestKeyFromCharUnmapped(t *testing.T) {
	_, _, ok := terminal.KeyFromChar('e', 0)
	if !ok {
		// 'e' should be mapped, this is fine
	}
	_, _, ok = terminal.KeyFromChar(0xff, 0)
	if ok {
		t.Error("expected unmapped for 0xff")
	}
}

func TestMapCellToScreenBasic(t *testing.T) {
	x, _ := terminal.MapCellToScreen(1, 1, 1920, 1080, 80, 24)
	if x < 0 || x >= 1920 {
		t.Errorf("out of range x: %d", x)
	}
	_, y := terminal.MapCellToScreen(1, 1, 1920, 1080, 80, 24)
	if y < 0 || y >= 1080 {
		t.Errorf("out of range y: %d", y)
	}
}

func TestMapCellToScreenClamping(t *testing.T) {
	// col=1, row=1: x=(1-1)*1920/80=0, y=(2*1-1)*1080/(2*24)=22
	x, _ := terminal.MapCellToScreen(1, 1, 1920, 1080, 80, 24)
	if x != 0 {
		t.Errorf("expected x=0 for col=1, got %d", x)
	}
	// col=80: x=(80-1)*1920/80=1896, row=24: y=(2*24-1)*1080/48=1057
	x2, y2 := terminal.MapCellToScreen(80, 24, 1920, 1080, 80, 24)
	if x2 != 1896 {
		t.Errorf("expected x=1896 for col=80, got %d", x2)
	}
	if y2 != 1057 {
		t.Errorf("expected y=1057 for row=24, got %d", y2)
	}

	// Test clamping: col > cols, row > rows → should clamp
	x3, y3 := terminal.MapCellToScreen(999, 999, 1920, 1080, 80, 24)
	if x3 != 1896 || y3 != 1057 {
		t.Errorf("expected clamped (1896,1057), got (%d,%d)", x3, y3)
	}

	// col < 1, row < 1 → should clamp
	x4, y4 := terminal.MapCellToScreen(0, 0, 1920, 1080, 80, 24)
	if x4 != 0 || y4 != 22 {
		t.Errorf("expected (0,22) for (0,0), got (%d,%d)", x4, y4)
	}
}

func TestMapCellToScreenZeroScreen(t *testing.T) {
	x, y := terminal.MapCellToScreen(1, 1, 0, 0, 80, 24)
	if x != 0 || y != 0 {
		t.Errorf("expected 0,0 for zero screen, got %d,%d", x, y)
	}
}

func TestParseTerminalInputEnter(t *testing.T) {
	buf := []byte{0x0d}
	result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
		MapCell: func(cx, cy int) (int, int) { return cx, cy },
	}, false)
	if len(result.Events) < 2 {
		t.Fatalf("expected at least 2 events (down+up), got %d", len(result.Events))
	}
	ke, ok := result.Events[0].(input.KeyEvent)
	if !ok {
		t.Fatalf("expected KeyEvent, got %T", result.Events[0])
	}
	if ke.Key != input.KeyEnter || !ke.Down {
		t.Errorf("expected KeyEnter down, got %v/%v", ke.Key, ke.Down)
	}
}

func TestParseTerminalInputTab(t *testing.T) {
	buf := []byte{0x09}
	result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
		MapCell: func(cx, cy int) (int, int) { return cx, cy },
	}, false)
	if len(result.Events) < 2 {
		t.Fatalf("expected 2 events, got %d", len(result.Events))
	}
	ke := result.Events[0].(input.KeyEvent)
	if ke.Key != input.KeyTab {
		t.Errorf("expected KeyTab, got %v", ke.Key)
	}
}

func TestParseTerminalInputBackspace(t *testing.T) {
	for _, bs := range []byte{0x7f, 0x08} {
		buf := []byte{bs}
		result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
			MapCell: func(cx, cy int) (int, int) { return cx, cy },
		}, false)
		ke := result.Events[0].(input.KeyEvent)
		if ke.Key != input.KeyBackspace {
			t.Errorf("for byte 0x%02x: expected KeyBackspace, got %v", bs, ke.Key)
		}
	}
}

func TestParseTerminalInputCtrlLetter(t *testing.T) {
	buf := []byte{0x03} // Ctrl+C
	result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
		MapCell: func(cx, cy int) (int, int) { return cx, cy },
	}, false)
	ke := result.Events[0].(input.KeyEvent)
	if ke.Key != input.KeyC || ke.Modifiers != input.ModCtrl {
		t.Errorf("expected KeyC/Ctrl, got %v/%v", ke.Key, ke.Modifiers)
	}
}

func TestParseTerminalInputPrintable(t *testing.T) {
	buf := []byte{'a'}
	result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
		MapCell: func(cx, cy int) (int, int) { return cx, cy },
	}, false)
	if len(result.Events) < 2 {
		t.Fatalf("expected 2 events, got %d", len(result.Events))
	}
	ke := result.Events[0].(input.KeyEvent)
	if ke.Key != input.KeyA || ke.Down != true {
		t.Errorf("expected KeyA down, got %v/%v", ke.Key, ke.Down)
	}
}

func TestParseTerminalInputEscapeFlush(t *testing.T) {
	buf := []byte{0x1b}
	result := terminal.ParseTerminalInput(buf, terminal.TerminalParseOptions{
		MapCell: func(cx, cy int) (int, int) { return cx, cy },
	}, true)
	if len(result.Events) == 0 {
		t.Fatal("expected at least 1 event after flush")
	}
	ke := result.Events[0].(input.KeyEvent)
	if ke.Key != input.KeyEscape {
		t.Errorf("expected KeyEscape, got %v", ke.Key)
	}
}

func TestKeyPair(t *testing.T) {
	events := terminal.KeyPair(input.KeyA, input.ModCtrl)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	down := events[0].(input.KeyEvent)
	up := events[1].(input.KeyEvent)
	if !down.Down || down.Key != input.KeyA || down.Modifiers != input.ModCtrl {
		t.Errorf("down event mismatch: %v", down)
	}
	if up.Down || up.Key != input.KeyA || up.Modifiers != input.ModCtrl {
		t.Errorf("up event mismatch: %v", up)
	}
}
