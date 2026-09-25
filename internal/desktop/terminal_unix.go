//go:build !windows

package desktop

import (
	"github.com/creack/pty"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func startTerminal(cmd *exec.Cmd, cols, rows int) (io.ReadWriteCloser, error) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	return pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func resizeTerminal(t io.ReadWriteCloser, cols, rows int) error {
	return pty.Setsize(t.(*os.File), &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}
func waitTerminal(cmd *exec.Cmd) error { return cmd.Wait() }
func stopTerminal(cmd *exec.Cmd)       { _ = cmd.Process.Signal(syscall.SIGTERM) }
