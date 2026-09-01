package app

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
)

var inBandMonitorScreenRestore = []byte("\x1b[2K\r")

type inBandMonitorState struct {
	panelID      string
	command      string
	marker       string
	startedAt    time.Time
	buffer       []byte
	sawBegin     bool
	pendingInput []byte
}

func (a *App) scheduleInBandMonitor(now time.Time) {
	if !a.canStartInBandMonitor() {
		return
	}
	for _, item := range a.panels.Panels() {
		if item.Kind != panel.KindMonitor && item.Kind != panel.KindTable && item.Kind != panel.KindLog {
			continue
		}
		if item.Command == "" || a.monitorActive[item.ID] {
			continue
		}
		interval := item.Interval
		if interval < time.Second {
			interval = time.Second
		}
		if last := a.monitorLast[item.ID]; !last.IsZero() && now.Sub(last) < interval {
			continue
		}
		if !policy.IsReadOnlyMonitorCommand(item.Command) {
			a.panels.SetError(item.ID, fmt.Errorf("%s", a.t("监控命令被只读策略拒绝")))
			a.monitorLast[item.ID] = now
			continue
		}
		a.startInBandMonitor(item, now)
		return
	}
}

func (a *App) canStartInBandMonitor() bool {
	return a.inBandMonitor == nil && len(a.monitorDeferredInput) == 0 && a.atPrompt && !a.sensitive &&
		!a.terminalDone && a.zmodem == nil && a.sshPTY != nil && len(a.lineInput) == 0 &&
		a.lineReliable && !a.aiLineCapture && !a.terminalPasteMode
}

func (a *App) startInBandMonitor(item *panel.Panel, now time.Time) {
	a.monitorSequence++
	marker := fmt.Sprintf("__AISSH_MONITOR_%d_", a.monitorSequence)
	state := &inBandMonitorState{
		panelID:   item.ID,
		command:   item.Command,
		marker:    marker,
		startedAt: now,
	}
	command := inBandMonitorCommand(marker, item.Command)
	a.inBandMonitor = state
	a.monitorActive[item.ID] = true
	a.atPrompt = false
	a.promptIdleSince = time.Time{}
	// The visible prompt is intentionally left untouched while the hidden query
	// runs. Make prompt detection wait for the fresh prompt emitted afterwards.
	a.outputTail.Append("\n[aissh monitor]\n")
	if _, err := a.sshPTY.Write([]byte(command)); err != nil {
		a.inBandMonitor = nil
		a.applyMonitorResult(monitorResultEvent{panelID: item.ID, err: err})
	}
}

func inBandMonitorCommand(marker, command string) string {
	// The complete begin/end markers are assembled by printf and do not appear
	// contiguously in the echoed command line. This lets the output filter safely
	// ignore shell echo and start collecting only after the command really runs.
	return "(__aissh_monitor_marker=" + quotePOSIXShell(marker) +
		"; printf '%sBEGIN__\\n' \"$__aissh_monitor_marker\"; sh -c " + quotePOSIXShell(strings.TrimSpace(command)) +
		"; __aissh_monitor_status=$?; printf '\\n%sEND__:%s\\033[2K\\r' \"$__aissh_monitor_marker\" \"$__aissh_monitor_status\")\r"
}

func quotePOSIXShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (a *App) processInBandMonitorOutput(data []byte) bool {
	state := a.inBandMonitor
	if state == nil {
		return false
	}
	visible, complete, content, err := state.consume(data)
	if !complete {
		return true
	}

	a.inBandMonitor = nil
	a.monitorDeferredInput = append(a.monitorDeferredInput, state.pendingInput...)
	if err == nil {
		content = a.monitor.Format(state.command, content)
	}
	a.applyMonitorResult(monitorResultEvent{panelID: state.panelID, content: content, err: err})
	if len(visible) > 0 {
		a.processSSHOutput(visible)
	}
	a.flushDeferredMonitorInput()
	return true
}

func (state *inBandMonitorState) consume(data []byte) (visible []byte, complete bool, content string, err error) {
	state.buffer = append(state.buffer, data...)
	beginMarker := []byte(state.marker + "BEGIN__")
	if !state.sawBegin {
		index := bytes.Index(state.buffer, beginMarker)
		if index < 0 {
			keep := len(beginMarker) - 1
			if keep > 0 && len(state.buffer) > keep {
				state.buffer = append(state.buffer[:0], state.buffer[len(state.buffer)-keep:]...)
			}
			return nil, false, "", nil
		}
		state.buffer = append(state.buffer[:0], state.buffer[index+len(beginMarker):]...)
		state.sawBegin = true
	}

	endMarker := []byte(state.marker + "END__:")
	endIndex := bytes.Index(state.buffer, endMarker)
	if endIndex < 0 {
		return nil, false, "", nil
	}
	statusStart := endIndex + len(endMarker)
	restoreOffset := bytes.Index(state.buffer[statusStart:], inBandMonitorScreenRestore)
	if restoreOffset < 0 {
		return nil, false, "", nil
	}
	restoreStart := statusStart + restoreOffset
	rawContent := trimInBandWrapperNewlines(state.buffer[:endIndex])
	statusText := strings.TrimSpace(string(state.buffer[statusStart:restoreStart]))
	status, parseErr := strconv.Atoi(statusText)
	visible = append([]byte(nil), state.buffer[restoreStart:]...)
	if parseErr != nil {
		return visible, true, "", fmt.Errorf("invalid monitor exit status %q", statusText)
	}
	if status != 0 {
		message := strings.TrimSpace(string(rawContent))
		if message == "" {
			message = fmt.Sprintf("monitor command exited with status %d", status)
		}
		return visible, true, "", fmt.Errorf("%s", message)
	}
	return visible, true, string(rawContent), nil
}

func trimInBandWrapperNewlines(value []byte) []byte {
	value = trimOneLeadingNewline(value)
	value = trimOneTrailingNewline(value)
	return value
}

func trimOneLeadingNewline(value []byte) []byte {
	if bytes.HasPrefix(value, []byte("\r\n")) {
		return value[2:]
	}
	if len(value) > 0 && (value[0] == '\r' || value[0] == '\n') {
		return value[1:]
	}
	return value
}

func trimOneTrailingNewline(value []byte) []byte {
	if bytes.HasSuffix(value, []byte("\r\n")) {
		return value[:len(value)-2]
	}
	if len(value) > 0 && (value[len(value)-1] == '\r' || value[len(value)-1] == '\n') {
		return value[:len(value)-1]
	}
	return value
}

func (a *App) checkInBandMonitorTimeout(now time.Time) {
	state := a.inBandMonitor
	if state == nil {
		return
	}
	timeout := a.monitor.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if now.Sub(state.startedAt) < timeout {
		return
	}
	a.inBandMonitor = nil
	a.monitorDeferredInput = append(a.monitorDeferredInput, state.pendingInput...)
	a.applyMonitorResult(monitorResultEvent{panelID: state.panelID, err: fmt.Errorf("monitor query timed out")})
	a.atPrompt = false
	if a.sshPTY != nil {
		_, _ = a.sshPTY.Write([]byte{0x03})
	}
}

func (a *App) flushDeferredMonitorInput() {
	if a.inBandMonitor != nil || !a.atPrompt || len(a.monitorDeferredInput) == 0 {
		return
	}
	data := append([]byte(nil), a.monitorDeferredInput...)
	a.monitorDeferredInput = nil
	a.handleInput(data)
}
