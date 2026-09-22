//go:build !windows

package monitor

import "os/exec"

func configureMonitorProcess(cmd *exec.Cmd) {}
