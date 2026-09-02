//go:build windows

package app

import (
	"golang.org/x/sys/windows"
)

type terminalState struct {
	inputMode uint32
}

func makeTerminalRaw(fd int) (*terminalState, error) {
	handle := windows.Handle(fd)
	var inputMode uint32
	if err := windows.GetConsoleMode(handle, &inputMode); err != nil {
		return nil, err
	}
	// Read Windows console input records directly instead of asking the VT input
	// layer to emulate a byte stream. In particular, this preserves committed
	// IME text in KEY_EVENT_RECORD.UnicodeChar.
	rawMode := inputMode &^ (windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_LINE_INPUT |
		windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_QUICK_EDIT_MODE |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	rawMode |= windows.ENABLE_EXTENDED_FLAGS | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_WINDOW_INPUT
	if err := windows.SetConsoleMode(handle, rawMode); err != nil {
		return nil, err
	}
	return &terminalState{inputMode: inputMode}, nil
}

func restoreTerminal(fd int, state *terminalState) error {
	return windows.SetConsoleMode(windows.Handle(fd), state.inputMode)
}
