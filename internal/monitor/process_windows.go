//go:build windows

package monitor

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureMonitorProcess(cmd *exec.Cmd) {
	// A background SSH query must not attach to the interactive console and
	// change the input mode used by the main application's console reader.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}
