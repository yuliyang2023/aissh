//go:build desktop && darwin

package main

// Wails' native file picker uses UTType on current macOS SDKs.

// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
