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

// maintainAIKeepalive protects long AI turns from both SSH transport idle
// timeouts and shell-level auto logout. Transport keepalive is configured when
// SSH starts; while the shell is at a known-safe empty prompt, an empty line
// resets shell timers such as Bash TMOUT without executing a command.
func (a *App) maintainAIKeepalive(now time.Time) bool {
	interval := a.config.AIKeepalive
	if interval <= 0 || !a.agentBusy || !a.atPrompt || a.sensitive || a.terminalDone || a.zmodem != nil || a.sshPTY == nil {
		return false
	}
	if a.aiLineCapture || len(a.lineInput) != 0 || !a.lineReliable {
		return false
	}
	if !a.lastAIKeepalive.IsZero() && now.Sub(a.lastAIKeepalive) < interval {
		return false
	}
	if _, err := a.sshPTY.Write(shellIdleKeepalive); err != nil {
		return false
	}
	a.lastAIKeepalive = now
	return true
}
