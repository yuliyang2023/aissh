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

func approvedCommandLineEnding() string {
	return "\n"
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
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		defer signal.Stop(resize)
		defer signal.Stop(stop)
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-stop:
				a.cancel()
				return
			case <-resize:
				a.events <- resizeEvent{}
			}
		}
	}()
}
