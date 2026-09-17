package app

import (
	"bytes"
	"strings"
)

var terminalClearSequences = [][]byte{
	{0x1b, '[', '2', 'J'},
	{0x1b, '[', '3', 'J'},
	{0x1b, 'c'},
}

// clearInlineTerminalOutput removes only the AI blocks inserted into the SSH
// viewport. The synchronized conversation in the right pane remains intact.
func (a *App) clearInlineTerminalOutput() {
	a.terminalResizePending = false
	a.inlineAI = nil
	a.inlineHistory = nil
	a.terminalRows = nil
	a.terminalScroll = 0
	a.lineAnchored = false
	a.dirty = true
}

func (a *App) observeTerminalClear(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	window := make([]byte, 0, len(a.clearCarry)+len(data))
	window = append(window, a.clearCarry...)
	window = append(window, data...)
	cleared := false
	for _, sequence := range terminalClearSequences {
		if bytes.Contains(window, sequence) {
			cleared = true
			break
		}
	}
	const carrySize = 3
	if len(window) > carrySize {
		a.clearCarry = append(a.clearCarry[:0], window[len(window)-carrySize:]...)
	} else {
		a.clearCarry = append(a.clearCarry[:0], window...)
	}
	if cleared {
		if a.terminalResizePending {
			// A resize repaint may span several reads. Preserve local blocks
			// until the next shell command; locally captured AI input can race
			// the repaint. Explicit clear/Ctrl+L still clears blocks directly.
			a.terminalRows = nil
			if a.inlineAI != nil {
				a.inlineAI.AwaitingPrompt = true
			}
		} else {
			a.clearInlineTerminalOutput()
		}
	}
	return cleared
}

func isTerminalClearCommand(line string) bool {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return false
	}
	command := fields[0]
	if command == "command" && len(fields) > 1 {
		command = fields[1]
	}
	switch command {
	case "clear", "/usr/bin/clear", "/bin/clear", "reset", "/usr/bin/reset":
		return true
	default:
		return false
	}
}
