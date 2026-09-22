//go:build windows

package app

import (
	"context"
	"runtime"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	notificationUser32          = windows.NewLazySystemDLL("user32.dll")
	notificationShell32         = windows.NewLazySystemDLL("shell32.dll")
	notificationKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	notificationRegisterClass   = notificationUser32.NewProc("RegisterClassExW")
	notificationDefWindowProc   = notificationUser32.NewProc("DefWindowProcW")
	notificationGetConsole      = notificationKernel32.NewProc("GetConsoleWindow")
	notificationGetModule       = notificationKernel32.NewProc("GetModuleHandleW")
	notificationIsWindow        = notificationUser32.NewProc("IsWindow")
	notificationIsIconic        = notificationUser32.NewProc("IsIconic")
	notificationShowWindow      = notificationUser32.NewProc("ShowWindowAsync")
	notificationSetForeground   = notificationUser32.NewProc("SetForegroundWindow")
	notificationCreateWindow    = notificationUser32.NewProc("CreateWindowExW")
	notificationDestroyWindow   = notificationUser32.NewProc("DestroyWindow")
	notificationLoadIcon        = notificationUser32.NewProc("LoadIconW")
	notificationPeekMessage     = notificationUser32.NewProc("PeekMessageW")
	notificationDispatchMessage = notificationUser32.NewProc("DispatchMessageW")
	notificationMessageBeep     = notificationUser32.NewProc("MessageBeep")
	notificationShellNotifyIcon = notificationShell32.NewProc("Shell_NotifyIconW")
)

const (
	notifyIconAdd          = 0
	notifyIconModify       = 1
	notifyIconDelete       = 2
	notifyIconSetVersion   = 4
	notifyIconMessage      = 0x01
	notifyIconImage        = 0x02
	notifyIconTip          = 0x04
	notifyIconInfo         = 0x10
	notifyInfo             = 0x01
	notifyNoSound          = 0x10
	notifyRespectQuietTime = 0x80
	notificationCallback   = 0x8001 // WM_APP + 1
	notificationIconID     = 1
	notifyBalloonClick     = 0x0405 // NIN_BALLOONUSERCLICK
	notifyIconSelect       = 0x0400 // NIN_SELECT
	notifyIconKeySelect    = 0x0401 // NIN_KEYSELECT
	// Bound the icon lifetime even when Windows suppresses the notification.
	notificationLifetime = 30 * time.Second
)

// Match NOTIFYICONDATAW and MSG, including pointer-sized fields and alignment.
type notificationIconData struct {
	Size            uint32
	Window          windows.Handle
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            windows.GUID
	BalloonIcon     windows.Handle
}

type notificationMessage struct {
	Window  windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

// WNDCLASSEXW. A real window procedure handles both sent and posted shell
// callbacks; checking only messages returned by PeekMessage misses sent ones.
type notificationWindowClass struct {
	Size, Style          uint32
	WindowProc           uintptr
	ClassExtra, WinExtra int32
	Instance             windows.Handle
	Icon, Cursor, Brush  windows.Handle
	MenuName, ClassName  *uint16
	SmallIcon            windows.Handle
}

var notificationActivations sync.Map // HWND -> func(); removed before destruction

// Go callback thunks cannot be freed. Register one for the process, rather than
// allocating another for every short-lived notification window.
var notificationWindowProc = windows.NewCallback(windowsAINotificationWindowProc)
var registerAINotificationClass = sync.OnceValues(func() (uintptr, uintptr) {
	name, _ := windows.UTF16PtrFromString("aissh.AINotification")
	instance, _, _ := notificationGetModule.Call(0)
	class := notificationWindowClass{
		WindowProc: notificationWindowProc,
		Instance:   windows.Handle(instance),
		ClassName:  name,
	}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, _ := notificationRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
	return atom, instance
})

func createAINotificationWindow(activate func()) uintptr {
	class, instance := registerAINotificationClass()
	if class == 0 {
		return 0
	}
	window, _, _ := notificationCreateWindow.Call(0, class, 0, 0,
		0, 0, 0, 0, 0, 0, instance, 0)
	if window != 0 {
		notificationActivations.Store(window, activate)
	}
	return window
}

func destroyAINotificationWindow(window uintptr) {
	notificationActivations.Delete(window)
	_, _, _ = notificationDestroyWindow.Call(window)
}

func windowsAINotificationWindowProc(window, message, wParam, lParam uintptr) uintptr {
	if message == notificationCallback {
		// Version 4 packs the icon ID into HIWORD(lParam); the legacy callback
		// uses wParam for the ID if NIM_SETVERSION was rejected by the shell.
		id := (lParam >> 16) & 0xffff
		if id == 0 {
			id = wParam
		}
		if id == notificationIconID {
			switch lParam & 0xffff {
			case notifyBalloonClick, notifyIconSelect, notifyIconKeySelect:
				if activate, ok := notificationActivations.Load(window); ok {
					activate.(func())()
				}
			}
		}
		return 0
	}
	result, _, _ := notificationDefWindowProc.Call(window, message, wParam, lParam)
	return result
}

func activateAIConsole(window uintptr) {
	if window == 0 {
		return
	}
	if valid, _, _ := notificationIsWindow.Call(window); valid == 0 {
		return
	}
	// Restore minimized consoles without unmaximizing an already visible one.
	if minimized, _, _ := notificationIsIconic.Call(window); minimized != 0 {
		_, _, _ = notificationShowWindow.Call(window, 9) // SW_RESTORE
	}
	_, _, _ = notificationSetForeground.Call(window)
}

type desktopAINotice struct{ title, message string }

func newAINotifier(ctx context.Context) func(string, string) {
	// Bind to this aissh session, never to whichever window happens to be in
	// the foreground when the AI finishes or when the user clicks its notice.
	console, _, _ := notificationGetConsole.Call()
	activate := func() {
		if ctx.Err() == nil {
			activateAIConsole(console)
		}
	}
	pending := make(chan desktopAINotice, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case notice := <-pending:
				if ctx.Err() == nil {
					runWindowsAINotifications(ctx, pending, notice, activate)
				}
			}
		}
	}()
	return func(title, message string) {
		if ctx.Err() != nil {
			return
		}
		// Never wait for the desktop shell; keep the latest pending result.
		select {
		case <-pending:
		default:
		}
		select {
		case pending <- desktopAINotice{title, message}:
		default:
		}
	}
}

func runWindowsAINotifications(ctx context.Context, pending <-chan desktopAINotice, notice desktopAINotice, activate func()) {
	// Window creation, message dispatch and destruction must use one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Sound remains available if Explorer cannot display a notification.
	_, _, _ = notificationMessageBeep.Call(0x40) // MB_ICONINFORMATION
	window := createAINotificationWindow(activate)
	if window == 0 {
		return
	}
	defer destroyAINotificationWindow(window)
	icon, _, _ := notificationLoadIcon.Call(0, 32516) // IDI_INFORMATION; shared system icon
	data := notificationIconData{
		Window: windows.Handle(window), ID: notificationIconID,
		Flags: notifyIconMessage | notifyIconImage | notifyIconTip, Icon: windows.Handle(icon), Version: 4,
		CallbackMessage: notificationCallback,
	}
	data.Size = uint32(unsafe.Sizeof(data))
	copyNotificationText(data.Tip[:], "aissh")
	if ok, _, _ := notificationShellNotifyIcon.Call(notifyIconAdd, uintptr(unsafe.Pointer(&data))); ok == 0 {
		return
	}
	defer func() {
		_, _, _ = notificationShellNotifyIcon.Call(notifyIconDelete, uintptr(unsafe.Pointer(&data)))
	}()
	_, _, _ = notificationShellNotifyIcon.Call(notifyIconSetVersion, uintptr(unsafe.Pointer(&data)))
	showWindowsAINotice(&data, notice)

	timer := time.NewTimer(notificationLifetime)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case notice = <-pending:
			_, _, _ = notificationMessageBeep.Call(0x40)
			showWindowsAINotice(&data, notice)
			timer.Reset(notificationLifetime)
		case <-ticker.C:
			var message notificationMessage
			for {
				ok, _, _ := notificationPeekMessage.Call(uintptr(unsafe.Pointer(&message)), window, 0, 0, 1) // PM_REMOVE
				if ok == 0 {
					break
				}
				_, _, _ = notificationDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
			}
		}
	}
}

func showWindowsAINotice(data *notificationIconData, notice desktopAINotice) {
	data.Flags = notifyIconInfo
	// MessageBeep supplies the sound once, independently of banner delivery.
	data.InfoFlags = notifyInfo | notifyNoSound | notifyRespectQuietTime
	copyNotificationText(data.InfoTitle[:], notice.title)
	copyNotificationText(data.Info[:], notice.message)
	_, _, _ = notificationShellNotifyIcon.Call(notifyIconModify, uintptr(unsafe.Pointer(data)))
}

func copyNotificationText(destination []uint16, value string) {
	clear(destination)
	if len(destination) == 0 {
		return
	}
	encoded := utf16.Encode([]rune(value))
	limit := len(destination) - 1 // Always leave a NUL terminator.
	if len(encoded) > limit {
		encoded = encoded[:limit]
		if len(encoded) > 0 && encoded[len(encoded)-1] >= 0xd800 && encoded[len(encoded)-1] <= 0xdbff {
			encoded = encoded[:len(encoded)-1]
		}
	}
	copy(destination, encoded)
}
