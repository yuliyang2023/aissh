package app

import (
	"bytes"
	"testing"
)

func TestWindowsConsoleIMETextEncodesAsUTF8(t *testing.T) {
	encoder := windowsInputEncoder{}
	var got []byte
	for _, value := range []rune("帮我检查 nginx") {
		got = append(got, encoder.encodeKey(windowsKeyInput{KeyDown: true, RepeatCount: 1, Char: value})...)
	}
	if want := []byte("帮我检查 nginx"); !bytes.Equal(got, want) {
		t.Fatalf("IME input=%q, want %q", got, want)
	}
}

func TestWindowsConsoleInputPreservesControlsAndSpecialKeys(t *testing.T) {
	encoder := windowsInputEncoder{}
	tests := []struct {
		name  string
		input windowsKeyInput
		want  string
	}{
		{"ctrl-shift-minus", windowsKeyInput{KeyDown: true, RepeatCount: 1, Char: 0x1f, ControlKeyState: windowsLeftCtrl | windowsShift}, "\x1f"},
		{"left", windowsKeyInput{KeyDown: true, RepeatCount: 1, VirtualKeyCode: windowsVKLeft}, "\x1b[D"},
		{"ctrl-left", windowsKeyInput{KeyDown: true, RepeatCount: 1, VirtualKeyCode: windowsVKLeft, ControlKeyState: windowsLeftCtrl}, "\x1b[1;5D"},
		{"delete", windowsKeyInput{KeyDown: true, RepeatCount: 1, VirtualKeyCode: windowsVKDelete}, "\x1b[3~"},
		{"shift-tab", windowsKeyInput{KeyDown: true, RepeatCount: 1, VirtualKeyCode: windowsVKTab, Char: '\t', ControlKeyState: windowsShift}, "\x1b[Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := string(encoder.encodeKey(test.input)); got != test.want {
				t.Fatalf("encoded key=%q, want %q", got, test.want)
			}
		})
	}
}

func TestWindowsConsoleInputJoinsSurrogatePair(t *testing.T) {
	encoder := windowsInputEncoder{}
	if got := encoder.encodeKey(windowsKeyInput{KeyDown: true, RepeatCount: 1, Char: 0xd83d}); len(got) != 0 {
		t.Fatalf("high surrogate produced output: %q", got)
	}
	if got := string(encoder.encodeKey(windowsKeyInput{KeyDown: true, RepeatCount: 1, Char: 0xde42})); got != "🙂" {
		t.Fatalf("surrogate pair=%q, want emoji", got)
	}
}

func TestWindowsConsoleMouseEncodesSGR(t *testing.T) {
	encoder := windowsInputEncoder{}
	press := encoder.encodeMouse(windowsMouseInput{X: 4, Y: 2, ButtonState: windowsMouseLeft})
	release := encoder.encodeMouse(windowsMouseInput{X: 4, Y: 2})
	wheel := encoder.encodeMouse(windowsMouseInput{X: 6, Y: 3, ButtonState: uint32(uint16(120)) << 16, EventFlags: windowsMouseWheel})
	if string(press) != "\x1b[<0;5;3M" || string(release) != "\x1b[<0;5;3m" || string(wheel) != "\x1b[<64;7;4M" {
		t.Fatalf("mouse press=%q release=%q wheel=%q", press, release, wheel)
	}
}

func TestWindowsConsoleMouseDropsHoverMovement(t *testing.T) {
	encoder := windowsInputEncoder{}
	if got := encoder.encodeMouse(windowsMouseInput{X: 12, Y: 8, EventFlags: windowsMouseMoved}); len(got) != 0 {
		t.Fatalf("hover movement encoded as %q", got)
	}

	got := encoder.encodeMouse(windowsMouseInput{X: 12, Y: 8, ButtonState: windowsMouseLeft, EventFlags: windowsMouseMoved})
	if want := "\x1b[<32;13;9M"; string(got) != want {
		t.Fatalf("drag movement=%q, want %q", got, want)
	}
}
