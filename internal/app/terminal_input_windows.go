//go:build windows

package app

import (
	"encoding/binary"
	"os"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsKeyEvent        = 0x0001
	windowsMouseEvent      = 0x0002
	windowsWindowSizeEvent = 0x0004
)

type windowsConsoleInputRecord struct {
	EventType uint16
	_         [2]byte
	Event     [16]byte
}

var readConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

func (a *App) startInputReader() {
	go func() {
		handle := windows.Handle(os.Stdin.Fd())
		records := make([]windowsConsoleInputRecord, 32)
		encoder := windowsInputEncoder{}
		for {
			count, err := readWindowsConsoleInput(handle, records)
			if err != nil {
				return
			}
			var keyboardAndMouse []byte
			debugging := a.inputDebug != nil
			keyRecords := 0
			hanRecords := 0
			actionableMouseRecords := 0
			encodedBytes := 0
			flush := func() {
				if len(keyboardAndMouse) == 0 {
					return
				}
				a.events <- inputEvent{data: append([]byte(nil), keyboardAndMouse...)}
				keyboardAndMouse = keyboardAndMouse[:0]
			}
			for _, record := range records[:count] {
				switch record.EventType {
				case windowsKeyEvent:
					input := decodeWindowsKeyInput(record.Event)
					if debugging && input.KeyDown {
						keyRecords++
						if unicode.Is(unicode.Han, input.Char) {
							hanRecords++
						}
					}
					encoded := encoder.encodeKey(input)
					if debugging {
						encodedBytes += len(encoded)
					}
					keyboardAndMouse = append(keyboardAndMouse, encoded...)
				case windowsMouseEvent:
					encoded := encoder.encodeMouse(decodeWindowsMouseInput(record.Event))
					if debugging && len(encoded) > 0 {
						actionableMouseRecords++
					}
					if debugging {
						encodedBytes += len(encoded)
					}
					keyboardAndMouse = append(keyboardAndMouse, encoded...)
				case windowsWindowSizeEvent:
					flush()
					a.events <- resizeEvent{}
				}
			}
			if debugging && (keyRecords > 0 || actionableMouseRecords > 0) {
				a.debugInput("win_console records=%d keys=%d han=%d mouse=%d encoded_bytes=%d", count, keyRecords, hanRecords, actionableMouseRecords, encodedBytes)
			}
			flush()
		}
	}()
}

func readWindowsConsoleInput(handle windows.Handle, records []windowsConsoleInputRecord) (int, error) {
	var count uint32
	result, _, callErr := readConsoleInputW.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&records[0])),
		uintptr(len(records)),
		uintptr(unsafe.Pointer(&count)),
	)
	if result == 0 {
		return 0, callErr
	}
	return int(count), nil
}

func decodeWindowsKeyInput(event [16]byte) windowsKeyInput {
	return windowsKeyInput{
		KeyDown:         binary.LittleEndian.Uint32(event[0:4]) != 0,
		RepeatCount:     binary.LittleEndian.Uint16(event[4:6]),
		VirtualKeyCode:  binary.LittleEndian.Uint16(event[6:8]),
		Char:            rune(binary.LittleEndian.Uint16(event[10:12])),
		ControlKeyState: binary.LittleEndian.Uint32(event[12:16]),
	}
}

func decodeWindowsMouseInput(event [16]byte) windowsMouseInput {
	return windowsMouseInput{
		X:               int(int16(binary.LittleEndian.Uint16(event[0:2]))),
		Y:               int(int16(binary.LittleEndian.Uint16(event[2:4]))),
		ButtonState:     binary.LittleEndian.Uint32(event[4:8]),
		ControlKeyState: binary.LittleEndian.Uint32(event[8:12]),
		EventFlags:      binary.LittleEndian.Uint32(event[12:16]),
	}
}
