package app

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
)

const inputDebugFilename = "aissh-input.log"

type inputDebugLogger struct {
	file   *os.File
	logger *log.Logger
}

func newInputDebugLogger(version string) (*inputDebugLogger, error) {
	path := filepath.Join(os.TempDir(), inputDebugFilename)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	debug := &inputDebugLogger{
		file:   file,
		logger: log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds),
	}
	executable, _ := os.Executable()
	debug.logger.Printf("start version=%q os=%s arch=%s pid=%d executable=%q", version, runtime.GOOS, runtime.GOARCH, os.Getpid(), executable)
	return debug, nil
}

func (a *App) debugInput(format string, values ...any) {
	if a.inputDebug != nil {
		a.inputDebug.logger.Printf(format, values...)
	}
}

func (a *App) closeInputDebug() {
	if a.inputDebug == nil {
		return
	}
	a.debugInput("stop")
	_ = a.inputDebug.file.Close()
}
