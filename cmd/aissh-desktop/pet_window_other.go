//go:build desktop && !darwin

package main

// Other platforms retain Wails' native AlwaysOnTop behaviour.
func configurePetWindow() {}
