//go:build !windows

package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

func supportsSSHControlPath() bool {
	return true
}

func interactiveSSHArgs(args []string) []string {
	return args
}

func startSSHSession(cmd *exec.Cmd, rows, cols int) (io.ReadWriteCloser, error) {
	return pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func resizeSSHSession(session io.ReadWriteCloser, rows, cols int) {
	terminal, ok := session.(*os.File)
	if !ok {
		return
	}
	_ = pty.Setsize(terminal, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func waitSSHProcess(_ context.Context, cmd *exec.Cmd) error {
	return cmd.Wait()
}

func stopSSHProcess(process *os.Process) {
	_ = process.Signal(syscall.SIGHUP)
}

func (a *App) startSignalReader() {
	resize := make(chan os.Signal, 2)
	signal.Notify(resize, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(resize)
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-resize:
				a.events <- resizeEvent{}
			}
		}
	}()
}
