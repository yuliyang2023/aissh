package app

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

// ConPTY can keep its output pipe open after SSH exits. Give its final output
// time to arrive without letting an open pipe prevent the application exiting.
const sshOutputDrainTimeout = time.Second

func (a *App) startOutputReader() {
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		buffer := make([]byte, 32*1024)
		for {
			count, err := a.sshPTY.Read(buffer)
			if count > 0 {
				select {
				case a.events <- outputEvent{data: append([]byte(nil), buffer[:count]...)}:
				case <-a.ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		// Only the process wait determines success. PTYs also report EOF or EIO
		// on a normal logout, and either can precede the process exit status.
		err := waitSSHProcess(a.ctx, a.sshCommand)
		timer := time.NewTimer(sshOutputDrainTimeout)
		defer timer.Stop()
		select {
		case <-outputDone:
		case <-timer.C:
		case <-a.ctx.Done():
			return
		}
		// Queue the exit after the final output so a fast connection failure
		// cannot discard the SSH diagnostic before the event loop sees it.
		select {
		case a.events <- sshExitEvent{err: err}:
		case <-a.ctx.Done():
		}
	}()
}

func (a *App) sshSessionError() error {
	if a.terminalErr == nil {
		return nil
	}
	message := a.t("SSH 连接失败或异常结束（%s）", a.parsed.Target)
	output := a.outputTail.String()
	if a.sshOutputTail != nil && a.sshOutputTail.String() != "" {
		output = a.sshOutputTail.String()
	}
	// The bounded output tail includes OpenSSH's connection/authentication
	// diagnostics. Strip terminal controls before replaying it outside the TUI.
	detail := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, trigger.StripANSI(output)))
	if detail == "" {
		return fmt.Errorf("%s: %w", message, a.terminalErr)
	}
	return fmt.Errorf("%s: %w\n%s", message, a.terminalErr, detail)
}
