package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func monitorScreenText(a *App) string {
	var text strings.Builder
	for y := 0; y < a.contentRows; y++ {
		for x := 0; x < a.mainCols; x++ {
			text.WriteRune(a.virtual.Cell(x, y).Char)
		}
		text.WriteByte('\n')
	}
	return text.String()
}

func TestRepeatedInBandMonitorsDoNotAddBlankLines(t *testing.T) {
	for _, bottom := range []bool{false, true} {
		for _, prefix := range []string{
			"\r\n", "\n", "\r\n\x1b[2K\r\x1b[28;1H",
			"\x1b[?25l\x1b[28;1H\x1b[32m",
			"\r\n\x1b]0;server\x07\x1b[28;1H",
			"\r\n\x1b]0;server\x1b\\\x1b[28;1H",
		} {
			for _, bytewise := range []bool{false, true} {
				t.Run(fmt.Sprintf("bottom=%v/prefix=%q/bytewise=%v", bottom, prefix, bytewise), func(t *testing.T) {
					a, terminal := newMonitorTestApp(t)
					if bottom {
						a.processSSHOutput([]byte("\r\n" + strings.Repeat("existing output\r\n", a.contentRows) + "root@server:~# "))
					}
					cursor := a.virtual.Cursor()
					a.inlineAI = &inlineAIState{Prompt: "检查服务", Response: "服务正常", Done: true, AnchorX: cursor.X, AnchorY: cursor.Y}
					answer := a.inlineAI
					before := monitorScreenText(a)
					history := len(a.terminalHistory)
					for tick := 0; tick < 40; tick++ {
						a.scheduleInBandMonitor(time.Now().Add(time.Duration(tick+1) * time.Minute))
						if a.inBandMonitor == nil {
							t.Fatalf("monitor stopped at tick %d", tick)
						}
						marker := a.inBandMonitor.marker
						response := marker + "BEGIN__\r\nHOST=server\r\n" + marker + "END__:0__" + prefix + "root@server:~# \x1b[0m\x1b[?25h"
						if bytewise {
							for _, b := range []byte(response) {
								a.processSSHOutput([]byte{b})
							}
						} else {
							a.processSSHOutput([]byte(response))
						}
						if a.inBandMonitor != nil || !a.atPrompt {
							t.Fatalf("monitor did not restore the shell at tick %d", tick)
						}
						after := a.virtual.Cursor()
						if after.X != cursor.X || after.Y != cursor.Y || monitorScreenText(a) != before || len(a.terminalHistory) != history {
							t.Fatalf("tick %d changed the terminal: cursor=(%d,%d) want=(%d,%d), history=%d want=%d", tick, after.X, after.Y, cursor.X, cursor.Y, len(a.terminalHistory), history)
						}
						if a.inlineAI != answer || answer.AnchorY != cursor.Y {
							t.Fatalf("monitor moved the AI reply at tick %d", tick)
						}
						terminal.Reset()
					}
				})
			}
		}
	}
}

func TestInBandMonitorRestorationPreservesRealShellBlankLines(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.scheduleMonitors()
	marker := a.inBandMonitor.marker
	a.processSSHOutput([]byte(marker + "BEGIN__\r\nHOST=server\r\n" + marker + "END__:0__"))
	a.processSSHOutput([]byte{'\r'})
	a.processSSHOutput([]byte{'\n'})
	a.processSSHOutput([]byte("root@server:~# "))
	terminal.Reset()
	a.handleInput([]byte("printf '\\n\\nDONE\\n'\r"))
	cursor := a.virtual.Cursor()
	a.processSSHOutput([]byte("\r\n\r\nDONE\r\nroot@server:~# "))
	if got := a.virtual.Cursor().Y; got != cursor.Y+3 {
		t.Fatalf("real shell blank lines were removed: row=%d want=%d", got, cursor.Y+3)
	}
	if !strings.Contains(monitorScreenText(a), "DONE") {
		t.Fatal("real command output was hidden by monitor cleanup")
	}
}
