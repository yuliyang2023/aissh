package app

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	windowsVKBack   = 0x08
	windowsVKTab    = 0x09
	windowsVKReturn = 0x0d
	windowsVKEscape = 0x1b
	windowsVKSpace  = 0x20
	windowsVKPrior  = 0x21
	windowsVKNext   = 0x22
	windowsVKEnd    = 0x23
	windowsVKHome   = 0x24
	windowsVKLeft   = 0x25
	windowsVKUp     = 0x26
	windowsVKRight  = 0x27
	windowsVKDown   = 0x28
	windowsVKInsert = 0x2d
	windowsVKDelete = 0x2e
	windowsVKF1     = 0x70
	windowsVKF12    = 0x7b
)

const (
	windowsRightAlt  = 0x0001
	windowsLeftAlt   = 0x0002
	windowsRightCtrl = 0x0004
	windowsLeftCtrl  = 0x0008
	windowsShift     = 0x0010
)

const (
	windowsMouseMoved    = 0x0001
	windowsMouseWheel    = 0x0004
	windowsMouseHWheel   = 0x0008
	windowsMouseLeft     = 0x0001
	windowsMouseRight    = 0x0002
	windowsMouseMiddle   = 0x0004
	windowsMouseBackward = 0x0008
	windowsMouseForward  = 0x0010
)

type windowsKeyInput struct {
	KeyDown         bool
	RepeatCount     uint16
	VirtualKeyCode  uint16
	Char            rune
	ControlKeyState uint32
}

type windowsMouseInput struct {
	X               int
	Y               int
	ButtonState     uint32
	ControlKeyState uint32
	EventFlags      uint32
}

type windowsInputEncoder struct {
	pendingHigh         rune
	previousMouseButton uint32
}

func (e *windowsInputEncoder) encodeKey(input windowsKeyInput) []byte {
	if !input.KeyDown {
		return nil
	}
	repeat := int(input.RepeatCount)
	if repeat < 1 {
		repeat = 1
	}
	// Windows can attach a tab character to Shift+Tab. Prefer the virtual key
	// for this modified special key so it remains distinguishable from Tab.
	if input.VirtualKeyCode == windowsVKTab && input.ControlKeyState&windowsShift != 0 {
		return encodeWindowsSpecialKey(input, repeat)
	}
	if input.Char != 0 {
		return e.encodeWindowsRune(input.Char, input.ControlKeyState, repeat)
	}
	if e.pendingHigh != 0 {
		prefix := utf8.AppendRune(nil, utf8.RuneError)
		e.pendingHigh = 0
		return append(prefix, encodeWindowsSpecialKey(input, repeat)...)
	}
	return encodeWindowsSpecialKey(input, repeat)
}

func (e *windowsInputEncoder) encodeWindowsRune(value rune, controlState uint32, repeat int) []byte {
	if value >= 0xd800 && value <= 0xdbff {
		e.pendingHigh = value
		return nil
	}
	if e.pendingHigh != 0 {
		if value >= 0xdc00 && value <= 0xdfff {
			value = utf16.DecodeRune(e.pendingHigh, value)
		} else {
			prefix := utf8.AppendRune(nil, utf8.RuneError)
			e.pendingHigh = 0
			return append(prefix, e.encodeWindowsRune(value, controlState, repeat)...)
		}
		e.pendingHigh = 0
	} else if value >= 0xdc00 && value <= 0xdfff {
		value = utf8.RuneError
	}
	unit := utf8.AppendRune(nil, value)
	alt := controlState&(windowsLeftAlt|windowsRightAlt) != 0
	ctrl := controlState&(windowsLeftCtrl|windowsRightCtrl) != 0
	altGr := controlState&windowsRightAlt != 0 && ctrl
	if alt && !altGr {
		unit = append([]byte{0x1b}, unit...)
	}
	return repeatBytes(unit, repeat)
}

func encodeWindowsSpecialKey(input windowsKeyInput, repeat int) []byte {
	shift := input.ControlKeyState&windowsShift != 0
	alt := input.ControlKeyState&(windowsLeftAlt|windowsRightAlt) != 0
	ctrl := input.ControlKeyState&(windowsLeftCtrl|windowsRightCtrl) != 0
	modifier := 1
	if shift {
		modifier++
	}
	if alt {
		modifier += 2
	}
	if ctrl {
		modifier += 4
	}
	modifiedFinal := func(final byte) []byte {
		if modifier == 1 {
			return []byte{0x1b, '[', final}
		}
		return []byte(fmt.Sprintf("\x1b[1;%d%c", modifier, final))
	}
	modifiedTilde := func(number int) []byte {
		if modifier == 1 {
			return []byte(fmt.Sprintf("\x1b[%d~", number))
		}
		return []byte(fmt.Sprintf("\x1b[%d;%d~", number, modifier))
	}
	var sequence []byte
	switch input.VirtualKeyCode {
	case windowsVKBack:
		sequence = []byte{0x08}
	case windowsVKTab:
		if shift {
			sequence = []byte("\x1b[Z")
		} else {
			sequence = []byte{'\t'}
		}
	case windowsVKReturn:
		sequence = []byte{'\r'}
	case windowsVKEscape:
		sequence = []byte{0x1b}
	case windowsVKSpace:
		if ctrl {
			sequence = []byte{0}
		} else {
			sequence = []byte{' '}
		}
	case windowsVKUp:
		sequence = modifiedFinal('A')
	case windowsVKDown:
		sequence = modifiedFinal('B')
	case windowsVKRight:
		sequence = modifiedFinal('C')
	case windowsVKLeft:
		sequence = modifiedFinal('D')
	case windowsVKHome:
		sequence = modifiedFinal('H')
	case windowsVKEnd:
		sequence = modifiedFinal('F')
	case windowsVKInsert:
		sequence = modifiedTilde(2)
	case windowsVKDelete:
		sequence = modifiedTilde(3)
	case windowsVKPrior:
		sequence = modifiedTilde(5)
	case windowsVKNext:
		sequence = modifiedTilde(6)
	case windowsVKF1, windowsVKF1 + 1, windowsVKF1 + 2, windowsVKF1 + 3:
		final := byte('P' + input.VirtualKeyCode - windowsVKF1)
		if modifier == 1 {
			sequence = []byte{0x1b, 'O', final}
		} else {
			sequence = []byte(fmt.Sprintf("\x1b[1;%d%c", modifier, final))
		}
	default:
		if input.VirtualKeyCode >= windowsVKF1+4 && input.VirtualKeyCode <= windowsVKF12 {
			numbers := [...]int{15, 17, 18, 19, 20, 21, 23, 24}
			sequence = modifiedTilde(numbers[input.VirtualKeyCode-(windowsVKF1+4)])
		}
	}
	return repeatBytes(sequence, repeat)
}

func repeatBytes(value []byte, count int) []byte {
	if len(value) == 0 || count <= 0 {
		return nil
	}
	result := make([]byte, 0, len(value)*count)
	for index := 0; index < count; index++ {
		result = append(result, value...)
	}
	return result
}

func (e *windowsInputEncoder) encodeMouse(input windowsMouseInput) []byte {
	// ENABLE_MOUSE_INPUT reports every hover movement. Passing those records to
	// the app can flood its event queue and make IME commits feel severely
	// delayed even though no UI action consumes them. Keep movement only while a
	// button is down, which is the only form needed for scrollbar dragging.
	if input.EventFlags&windowsMouseMoved != 0 && input.ButtonState&0x1f == 0 {
		return nil
	}
	modifiers := 0
	if input.ControlKeyState&windowsShift != 0 {
		modifiers |= 4
	}
	if input.ControlKeyState&(windowsLeftAlt|windowsRightAlt) != 0 {
		modifiers |= 8
	}
	if input.ControlKeyState&(windowsLeftCtrl|windowsRightCtrl) != 0 {
		modifiers |= 16
	}
	button, release, ok := 0, false, false
	switch {
	case input.EventFlags&windowsMouseWheel != 0:
		button, ok = 64, true
		if int16(input.ButtonState>>16) < 0 {
			button = 65
		}
	case input.EventFlags&windowsMouseHWheel != 0:
		button, ok = 66, true
		if int16(input.ButtonState>>16) < 0 {
			button = 67
		}
	case input.EventFlags&windowsMouseMoved != 0:
		button, ok = windowsMouseButtonCode(input.ButtonState), true
		button |= 32
	default:
		changed := (e.previousMouseButton ^ input.ButtonState) & 0x1f
		if changed != 0 {
			pressed := changed & input.ButtonState
			if pressed != 0 {
				button = windowsMouseButtonCode(pressed)
			} else {
				button = windowsMouseButtonCode(changed & e.previousMouseButton)
				release = true
			}
			ok = true
		}
	}
	e.previousMouseButton = input.ButtonState & 0x1f
	if !ok {
		return nil
	}
	terminator := 'M'
	if release {
		terminator = 'm'
	}
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", button|modifiers, input.X+1, input.Y+1, terminator))
}

func windowsMouseButtonCode(buttons uint32) int {
	switch {
	case buttons&windowsMouseLeft != 0:
		return 0
	case buttons&windowsMouseMiddle != 0:
		return 1
	case buttons&windowsMouseRight != 0:
		return 2
	case buttons&windowsMouseBackward != 0:
		return 3
	case buttons&windowsMouseForward != 0:
		return 3
	default:
		return 3
	}
}
