//go:build windows

package app

import (
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestNotificationWindowsABI(t *testing.T) {
	var data notificationIconData
	iconSize, infoOffset, titleOffset, messageSize := uintptr(976), uintptr(304), uintptr(820), uintptr(48)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		iconSize, infoOffset, titleOffset, messageSize = 956, 288, 804, 32
	}
	if unsafe.Sizeof(data) != iconSize || unsafe.Offsetof(data.Info) != infoOffset ||
		unsafe.Offsetof(data.InfoTitle) != titleOffset || unsafe.Sizeof(notificationMessage{}) != messageSize {
		t.Fatal("notification structures do not match the Windows ABI")
	}
}

func TestNotificationUTF16Truncation(t *testing.T) {
	for _, test := range []struct {
		value string
		size  int
		want  string
	}{
		{"完成🙂", 5, "完成🙂"},
		{"完成🙂", 4, "完成"},
		{"done", 1, ""},
		{"done", 0, ""},
	} {
		buffer := make([]uint16, test.size)
		for index := range buffer {
			buffer[index] = 'x'
		}
		copyNotificationText(buffer, test.value)
		end := 0
		for end < len(buffer) && buffer[end] != 0 {
			end++
		}
		if got := string(utf16.Decode(buffer[:end])); got != test.want || (len(buffer) > 0 && buffer[len(buffer)-1] != 0) {
			t.Fatalf("encoded %q into %d units: %q", test.value, test.size, got)
		}
	}
}
