//go:build windows

package desktop

import (
	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"syscall"
)

func startTerminal(cmd *exec.Cmd, cols, rows int) (io.ReadWriteCloser, error) {
	t, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, err
	}
	pid, handle, err := t.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{Dir: cmd.Dir, Env: cmd.Env, Sys: cmd.SysProcAttr})
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	_ = windows.CloseHandle(windows.Handle(handle))
	cmd.Process, err = os.FindProcess(pid)
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	return t, nil
}
func resizeTerminal(t io.ReadWriteCloser, cols, rows int) error {
	return t.(interface{ Resize(int, int) error }).Resize(cols, rows)
}
func waitTerminal(cmd *exec.Cmd) error {
	state, err := cmd.Process.Wait()
	if err != nil {
		return err
	}
	if !state.Success() {
		return &exec.ExitError{ProcessState: state}
	}
	return nil
}
func stopTerminal(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
