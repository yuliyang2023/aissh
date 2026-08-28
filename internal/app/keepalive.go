package app

import (
	"strconv"
	"time"
)

var shellIdleKeepalive = []byte{'\r'}

func sshServerAliveOptions(interval time.Duration) []string {
	if interval <= 0 {
		return nil
	}
	seconds := int((interval + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return []string{
		"-o", "ServerAliveInterval=" + strconv.Itoa(seconds),
		"-o", "ServerAliveCountMax=3",
	}
}

// shellKeepaliveDelay leaves ten percent of the configured TMOUT as a safety
// margin for scheduler and SSH latency. The caller only uses this while AI owns
// an otherwise idle shell prompt.
func shellKeepaliveDelay(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 0
	}
	delay := timeout * 9 / 10
	if delay <= 0 {
		return timeout
	}
	return delay
}

// maintainAIKeepalive protects a known-safe empty remote shell prompt from
// shell-level auto logout while AI is running. SSH transport keepalive is
// configured independently when SSH starts. Completing one empty input line is
// the least invasive reliable way to reset timers such as Bash TMOUT.
func (a *App) maintainAIKeepalive(now time.Time) bool {
	timeout := a.config.ShellTMOUT
	if timeout <= 0 || !a.agentBusy || !a.atPrompt || a.sensitive || a.terminalDone || a.zmodem != nil || a.sshPTY == nil {
		return false
	}
	if a.aiLineCapture || len(a.lineInput) != 0 || !a.lineReliable {
		return false
	}
	if a.promptIdleSince.IsZero() {
		a.promptIdleSince = now
		return false
	}
	if now.Sub(a.promptIdleSince) < shellKeepaliveDelay(timeout) {
		return false
	}
	if _, err := a.sshPTY.Write(shellIdleKeepalive); err != nil {
		return false
	}
	// Wait for the resulting prompt before starting the next TMOUT window.
	a.atPrompt = false
	a.promptIdleSince = time.Time{}
	return true
}
