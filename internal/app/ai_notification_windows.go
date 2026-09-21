//go:build windows

package app

import (
	"context"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	notificationUser32          = windows.NewLazySystemDLL("user32.dll")
	notificationShell32         = windows.NewLazySystemDLL("shell32.dll")
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
	notifyIconImage        = 0x02
	notifyIconTip          = 0x04
	notifyIconInfo         = 0x10
	notifyInfo             = 0x01
	notifyNoSound          = 0x10
	notifyRespectQuietTime = 0x80
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

type desktopAINotice struct{ title, message string }

func newAINotifier(ctx context.Context) func(string, string) {
	pending := make(chan desktopAINotice, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case notice := <-pending:
				if ctx.Err() == nil {
					runWindowsAINotifications(ctx, pending, notice)
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

func runWindowsAINotifications(ctx context.Context, pending <-chan desktopAINotice, notice desktopAINotice) {
	// Window creation, message dispatch and destruction must use one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Sound remains available if Explorer cannot display a notification.
	_, _, _ = notificationMessageBeep.Call(0x40) // MB_ICONINFORMATION
	class, _ := windows.UTF16PtrFromString("STATIC")
	window, _, _ := notificationCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0)
	if window == 0 {
		return
	}
	defer notificationDestroyWindow.Call(window)
	icon, _, _ := notificationLoadIcon.Call(0, 32516) // IDI_INFORMATION; shared system icon
	data := notificationIconData{
		Window: windows.Handle(window), ID: 1,
		Flags: notifyIconImage | notifyIconTip, Icon: windows.Handle(icon), Version: 4,
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
