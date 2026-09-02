//go:build !windows

package app

import "os"

func (a *App) startInputReader() {
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := os.Stdin.Read(buffer)
			if count > 0 {
				a.events <- inputEvent{data: append([]byte(nil), buffer[:count]...)}
			}
			if err != nil {
				return
			}
		}
	}()
}
