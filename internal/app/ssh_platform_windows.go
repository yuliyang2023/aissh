//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func supportsSSHControlPath() bool {
	return false
}

func interactiveSSHArgs(args []string) []string {
	return args
}

func approvedCommandLineEnding() string {
	return "\r"
}

func startSSHSession(cmd *exec.Cmd, rows, cols int) (io.ReadWriteCloser, error) {
	terminal, err := conpty.New(cols, rows, 0)
	if err != nil {
		return nil, fmt.Errorf("create Windows ConPTY: %w", err)
	}
	processAttributes := &syscall.ProcAttr{
		Dir: cmd.Dir,
		Env: cmd.Env,
		Sys: cmd.SysProcAttr,
	}
	pid, handle, err := terminal.Spawn(cmd.Path, cmd.Args, processAttributes)
	if err != nil {
		_ = terminal.Close()
		return nil, fmt.Errorf("start SSH in Windows ConPTY: %w", err)
	}
	process, processErr := os.FindProcess(pid)
	_ = windows.CloseHandle(windows.Handle(handle))
	if processErr != nil {
		_ = terminal.Close()
		return nil, fmt.Errorf("open Windows SSH process: %w", processErr)
	}
	cmd.Process = process
	return terminal, nil
}

func resizeSSHSession(session io.ReadWriteCloser, rows, cols int) {
	resizable, ok := session.(interface{ Resize(int, int) error })
	if !ok {
		return
	}
	_ = resizable.Resize(cols, rows)
}

func waitSSHProcess(ctx context.Context, cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return errors.New("SSH process was not started")
	}
	type processResult struct {
		state *os.ProcessState
		err   error
	}
	done := make(chan processResult, 1)
	go func() {
		state, err := cmd.Process.Wait()
		done <- processResult{state: state, err: err}
	}()

	var result processResult
	select {
	case result = <-done:
	case <-ctx.Done():
		killErr := terminateWindowsProcess(cmd.Process)
		result = <-done
		cmd.ProcessState = result.state
		if killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return errors.Join(ctx.Err(), killErr)
		}
		return ctx.Err()
	}

	cmd.ProcessState = result.state
	if result.err != nil {
		return result.err
	}
	if result.state != nil && !result.state.Success() {
		return &exec.ExitError{ProcessState: result.state}
	}
	return nil
}

func stopSSHProcess(process *os.Process) {
	_ = terminateWindowsProcess(process)
}

func terminateWindowsProcess(process *os.Process) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

func (a *App) startSignalReader() {
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastCols, lastRows := a.cols, a.rows
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil && (cols != lastCols || rows != lastRows) {
					lastCols, lastRows = cols, rows
					a.events <- resizeEvent{}
				}
			}
		}
	}()
}
