//go:build desktop && darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void configurePetNativeWindow(void);
*/
import "C"

// Wails' AlwaysOnTop only sets the window level, not its Spaces behaviour.
func configurePetWindow() {
	C.configurePetNativeWindow()
}
