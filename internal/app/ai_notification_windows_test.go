//go:build windows

package app

import (
	"runtime"
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
	var class notificationWindowClass
	classSize, instanceOffset, nameOffset := uintptr(80), uintptr(24), uintptr(64)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		classSize, instanceOffset, nameOffset = 48, 20, 40
	}
	if unsafe.Sizeof(class) != classSize || unsafe.Offsetof(class.Instance) != instanceOffset ||
		unsafe.Offsetof(class.ClassName) != nameOffset {
		t.Fatal("notification window class does not match the Windows ABI")
	}
}

func TestNotificationClickRoutesToItsSession(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var firstCalls, secondCalls int
	first := createAINotificationWindow(func() { firstCalls++ })
	if first == 0 {
		t.Fatal("could not create first notification window")
	}
	defer destroyAINotificationWindow(first)
	second := createAINotificationWindow(func() { secondCalls++ })
	if second == 0 {
		t.Fatal("could not create second notification window")
	}
	defer destroyAINotificationWindow(second)
	send := notificationUser32.NewProc("SendMessageW")
	for _, test := range []struct {
		name          string
		window        uintptr
		wParam        uintptr
		lParam        uintptr
		first, second int
	}{
		{"first balloon", first, 0, notificationIconID<<16 | notifyBalloonClick, 1, 0},
		{"second balloon", second, 0, notificationIconID<<16 | notifyBalloonClick, 1, 1},
		{"legacy balloon", first, notificationIconID, notifyBalloonClick, 2, 1},
		{"tray selection", second, 0, notificationIconID<<16 | notifyIconSelect, 2, 2},
		{"keyboard selection", second, 0, notificationIconID<<16 | notifyIconKeySelect, 2, 3},
		{"balloon shown", first, 0, notificationIconID<<16 | 0x402, 2, 3},
		{"balloon dismissed", first, 0, notificationIconID<<16 | 0x403, 2, 3},
		{"balloon timeout", first, 0, notificationIconID<<16 | 0x404, 2, 3},
		{"right click", first, 0, notificationIconID<<16 | 0x205, 2, 3},
		{"other icon", first, 0, 2<<16 | notifyBalloonClick, 2, 3},
		{"other legacy icon", second, 2, notifyBalloonClick, 2, 3},
	} {
		// Stay on the window's OS thread: a subtest goroutine sending a
		// synchronous message would wait for this thread to pump messages.
		// SendMessage exercises callbacks that never appear as a queued MSG.
		_, _, _ = send.Call(test.window, notificationCallback, test.wParam, test.lParam)
		if firstCalls != test.first || secondCalls != test.second {
			t.Fatalf("%s: activations=(%d, %d), want (%d, %d)", test.name, firstCalls, secondCalls, test.first, test.second)
		}
	}
	post := notificationUser32.NewProc("PostMessageW")
	if ok, _, err := post.Call(first, notificationCallback, 0, notificationIconID<<16|notifyBalloonClick); ok == 0 {
		t.Fatalf("post notification click: %v", err)
	}
	var message notificationMessage
	for {
		ok, _, _ := notificationPeekMessage.Call(uintptr(unsafe.Pointer(&message)), first, 0, 0, 1)
		if ok == 0 {
			break
		}
		_, _, _ = notificationDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
	if firstCalls != 3 || secondCalls != 3 {
		t.Fatalf("queued click activations=(%d, %d), want (3, 3)", firstCalls, secondCalls)
	}
}

func TestNotificationWindowReleasesActivation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for range 3 {
		window := createAINotificationWindow(func() { t.Fatal("unexpected activation") })
		if window == 0 {
			t.Fatal("could not create notification window")
		}
		destroyAINotificationWindow(window)
		if _, exists := notificationActivations.Load(window); exists {
			t.Fatal("destroyed notification retained its session callback")
		}
		if valid, _, _ := notificationIsWindow.Call(window); valid != 0 {
			t.Fatal("notification window was not destroyed")
		}
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
