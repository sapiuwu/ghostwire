package terminal

import (
	"ghostwire/internal/domain/input"
)

type TerminalParseOptions struct {
	MapCell func(cellX, cellY int) (int, int)
}

type TerminalParseResult struct {
	Events   []input.InputEvent
	Consumed int
}

var pasteStart = []byte{0x1b, 0x5b, 0x32, 0x30, 0x30, 0x7e}
var pasteEnd = []byte{0x1b, 0x5b, 0x32, 0x30, 0x31, 0x7e}

var shiftedSymbols = map[byte]input.Key{
	'!': input.KeyDigit1, '@': input.KeyDigit2, '#': input.KeyDigit3,
	'$': input.KeyDigit4, '%': input.KeyDigit5, '^': input.KeyDigit6,
	'&': input.KeyDigit7, '*': input.KeyDigit8, '(': input.KeyDigit9,
	')': input.KeyDigit0, '_': input.KeyOemMinus, '+': input.KeyOemPlus,
	'{': input.KeyOem4, '}': input.KeyOem6, '|': input.KeyOem5,
	':': input.KeyOem1, '"': input.KeyOem7, '<': input.KeyOemComma,
	'>': input.KeyOemPeriod, '?': input.KeyOem2, '~': input.KeyOem3,
}

var plainSymbols = map[byte]input.Key{
	'-': input.KeyOemMinus, '=': input.KeyOemPlus, '[': input.KeyOem4,
	']': input.KeyOem6, '\\': input.KeyOem5, ';': input.KeyOem1,
	'\'': input.KeyOem7, ',': input.KeyOemComma, '.': input.KeyOemPeriod,
	'/': input.KeyOem2, '`': input.KeyOem3,
}

var csiKeys = map[byte]input.Key{
	'A': input.KeyUp, 'B': input.KeyDown, 'C': input.KeyRight, 'D': input.KeyLeft,
	'H': input.KeyHome, 'F': input.KeyEnd,
}

var tildeKeys = map[int]input.Key{
	1: input.KeyHome, 2: input.KeyInsert, 3: input.KeyDelete, 4: input.KeyEnd,
	5: input.KeyPageUp, 6: input.KeyPageDown, 7: input.KeyHome, 8: input.KeyEnd,
	11: input.KeyF1, 12: input.KeyF2, 13: input.KeyF3, 14: input.KeyF4,
	15: input.KeyF5, 17: input.KeyF6, 18: input.KeyF7, 19: input.KeyF8,
	20: input.KeyF9, 21: input.KeyF10, 23: input.KeyF11, 24: input.KeyF12,
}

var ss3Keys = map[byte]input.Key{
	'P': input.KeyF1, 'Q': input.KeyF2, 'R': input.KeyF3, 'S': input.KeyF4,
	'A': input.KeyUp, 'B': input.KeyDown, 'C': input.KeyRight, 'D': input.KeyLeft,
	'H': input.KeyHome, 'F': input.KeyEnd,
}

func KeyPair(key input.Key, modifiers input.Modifier) []input.InputEvent {
	return []input.InputEvent{
		input.KeyEvent{Key: key, Down: true, Modifiers: modifiers},
		input.KeyEvent{Key: key, Down: false, Modifiers: modifiers},
	}
}

func KeyFromChar(ch byte, baseModifiers input.Modifier) (input.Key, input.Modifier, bool) {
	if ch >= 'a' && ch <= 'z' {
		return input.KeyA + input.Key(ch-'a'), baseModifiers, true
	}
	if ch >= 'A' && ch <= 'Z' {
		return input.KeyA + input.Key(ch-'A'), baseModifiers | input.ModShift, true
	}
	if ch >= '0' && ch <= '9' {
		return input.KeyDigit0 + input.Key(ch-'0'), baseModifiers, true
	}
	if ch == ' ' {
		return input.KeySpace, baseModifiers, true
	}
	if k, ok := shiftedSymbols[ch]; ok {
		return k, baseModifiers | input.ModShift, true
	}
	if k, ok := plainSymbols[ch]; ok {
		return k, baseModifiers, true
	}
	return 0, 0, false
}

func MapCellToScreen(cellX, cellY, screenW, screenH, cols, rows int) (int, int) {
	if screenW <= 0 || screenH <= 0 || cols <= 0 || rows <= 0 {
		return 0, 0
	}
	col := cellX
	if col < 1 {
		col = 1
	}
	if col > cols {
		col = cols
	}
	row := cellY
	if row < 1 {
		row = 1
	}
	if row > rows {
		row = rows
	}
	x := (col - 1) * screenW / cols
	y := (2*row - 1) * screenH / (2 * rows)
	if x > screenW-1 {
		x = screenW - 1
	}
	if y > screenH-1 {
		y = screenH - 1
	}
	return x, y
}

func decodeXtermMods(value int) input.Modifier {
	if value < 1 {
		return 0
	}
	flags := value - 1
	var mods input.Modifier
	if flags&1 != 0 {
		mods |= input.ModShift
	}
	if flags&2 != 0 {
		mods |= input.ModAlt
	}
	if flags&4 != 0 {
		mods |= input.ModCtrl
	}
	if flags&8 != 0 {
		mods |= input.ModMeta
	}
	return mods
}

func ParseTerminalInput(buf []byte, options TerminalParseOptions, flush bool) TerminalParseResult {
	events, consumed := parsePlain(buf, 0, len(buf), options, flush)
	return TerminalParseResult{Events: events, Consumed: consumed}
}

func parsePlain(buf []byte, from, to int, options TerminalParseOptions, flush bool) ([]input.InputEvent, int) {
	var events []input.InputEvent
	i := from
	for i < to {
		b := buf[i]
		if b == 0x1b {
			parsed, next, ok := parseEscape(buf, i, to, options, flush)
			if !ok {
				break
			}
			events = append(events, parsed...)
			i = next
			continue
		}
		if b == 0x7f || b == 0x08 {
			events = append(events, KeyPair(input.KeyBackspace, 0)...)
			i++
			continue
		}
		if b == 0x09 {
			events = append(events, KeyPair(input.KeyTab, 0)...)
			i++
			continue
		}
		if b == 0x0d || b == 0x0a {
			events = append(events, KeyPair(input.KeyEnter, 0)...)
			i++
			continue
		}
		if b == 0x00 {
			events = append(events, KeyPair(input.KeySpace, input.ModCtrl)...)
			i++
			continue
		}
		if b < 0x20 {
			if b >= 1 && b <= 26 {
				events = append(events, KeyPair(input.KeyA+input.Key(b-1), input.ModCtrl)...)
			}
			i++
			continue
		}
		if b < 0x80 {
			if k, mods, ok := KeyFromChar(b, 0); ok {
				events = append(events, KeyPair(k, mods)...)
			}
			i++
			continue
		}
		// UTF-8: skip for now
		i++
	}
	return events, i
}

func parseEscape(buf []byte, start, limit int, options TerminalParseOptions, flush bool) ([]input.InputEvent, int, bool) {
	if start+1 >= limit {
		if flush {
			return KeyPair(input.KeyEscape, 0), start + 1, true
		}
		return nil, 0, false
	}
	nextByte := buf[start+1]
	if nextByte == 0x1b {
		return KeyPair(input.KeyEscape, 0), start + 1, true
	}
	if nextByte == 0x5b {
		return parseCsi(buf, start+1, limit, options, flush)
	}
	if nextByte == 0x4f {
		if start+2 >= limit {
			if flush {
				return nil, limit, true
			}
			return nil, 0, false
		}
		key := buf[start+2]
		if k, ok := ss3Keys[key]; ok {
			return KeyPair(k, 0), start + 3, true
		}
		return nil, start + 3, true
	}
	// Alt+char
	if nextByte < 0x80 {
		if k, mods, ok := KeyFromChar(nextByte, input.ModAlt); ok {
			return KeyPair(k, mods), start + 2, true
		}
		return nil, start + 2, true
	}
	return nil, start + 2, true
}

func parseCsi(buf []byte, start, limit int, options TerminalParseOptions, flush bool) ([]input.InputEvent, int, bool) {
	i := start + 1
	for i < limit {
		b := buf[i]
		if b >= 0x40 && b <= 0x7e {
			break
		}
		if i-start > 64 {
			return nil, limit, true
		}
		i++
	}
	if i >= limit {
		if flush {
			return nil, limit, true
		}
		return nil, 0, false
	}
	final := buf[i]
	params := string(buf[start+1 : i])
	next := i + 1

	// SGR mouse
	if len(params) > 0 && params[0] == '<' {
		if final == 'M' || final == 'm' {
			events := parseSgrMouse(params, final == 'M', options)
			return events, next, true
		}
		return nil, next, true
	}

	// Bracketed paste
	if params == "200" && final == '~' {
		end := findSequence(buf, i+1, limit, pasteEnd)
		if end < 0 {
			if !flush {
				return nil, 0, false
			}
			inner, _ := parsePlain(buf, i+1, limit, options, true)
			return inner, limit, true
		}
		inner, _ := parsePlain(buf, i+1, end, options, true)
		return inner, end + len(pasteEnd), true
	}

	// Focus
	if (final == 'I' || final == 'O') && params == "" {
		return nil, next, true
	}
	// Reverse tab
	if final == 'Z' {
		return KeyPair(input.KeyTab, input.ModShift), next, true
	}

	// Parse params
	nums := parseInts(params)
	modVal := 0
	if len(nums) > 1 {
		modVal = nums[1]
	}

	if k, ok := csiKeys[final]; ok {
		return KeyPair(k, decodeXtermMods(modVal)), next, true
	}
	if final == '~' {
		if len(nums) > 0 {
			if k, ok := tildeKeys[nums[0]]; ok {
				return KeyPair(k, decodeXtermMods(modVal)), next, true
			}
		}
		return nil, next, true
	}
	return nil, next, true
}

func parseSgrMouse(params string, press bool, options TerminalParseOptions) []input.InputEvent {
	parts := parseInts(params)
	if len(parts) < 3 {
		return nil
	}
	buttonBits := parts[0]
	cellX := parts[1]
	cellY := parts[2]

	var mods input.Modifier
	if buttonBits&4 != 0 {
		mods |= input.ModShift
	}
	if buttonBits&8 != 0 {
		mods |= input.ModAlt
	}
	if buttonBits&16 != 0 {
		mods |= input.ModCtrl
	}

	x, y := options.MapCell(cellX, cellY)

	if buttonBits&64 != 0 {
		direction := buttonBits & 3
		dy := int16(120)
		if direction != 0 {
			dy = -120
		}
		return []input.InputEvent{input.WheelEvent{X: int32(x), Y: int32(y), DeltaY: dy, Modifiers: mods}}
	}
	if buttonBits&32 != 0 {
		return []input.InputEvent{input.MouseMoveEvent{X: int32(x), Y: int32(y), Modifiers: mods}}
	}
	button := buttonBits & 3
	if button == 3 {
		if press {
			return []input.InputEvent{input.MouseMoveEvent{X: int32(x), Y: int32(y), Modifiers: mods}}
		}
		return nil
	}
	mappedButton := input.MouseButton(button)
	if button == 1 {
		mappedButton = input.MouseMiddle
	} else if button == 2 {
		mappedButton = input.MouseRight
	}
	return []input.InputEvent{input.MouseButtonEvent{Button: mappedButton, Down: press, X: int32(x), Y: int32(y), Modifiers: mods}}
}

func parseInts(s string) []int {
	var result []int
	current := 0
	hasNum := false
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			current = current*10 + int(s[i]-'0')
			hasNum = true
		} else {
			if hasNum {
				result = append(result, current)
				current = 0
				hasNum = false
			}
		}
	}
	if hasNum {
		result = append(result, current)
	}
	return result
}

func findSequence(buf []byte, from, limit int, seq []byte) int {
	for i := from; i+len(seq) <= limit; i++ {
		match := true
		for j := 0; j < len(seq); j++ {
			if buf[i+j] != seq[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
