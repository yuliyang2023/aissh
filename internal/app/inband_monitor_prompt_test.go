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
	return strings.ReplaceAll(text.String(), "\ue000", "")
}

func TestRepeatedMonitorsPreserveScreenAcrossChunkedPrompts(t *testing.T) {
	for _, bytewise := range []bool{false, true} {
		t.Run(fmt.Sprint(bytewise), func(t *testing.T) {
			a, _ := newMonitorTestApp(t)
			a.panels.ByID("system").Command = "printf 'HOST=server\\n'"
			a.ensureMonitorAudit(a.panelMonitorAudit(a.panels.ByID("system")))
			a.approveProposal()
			var firstScreen string
			var firstHistory int
			for tick := 0; tick < 20; tick++ {
				a.scheduleInBandMonitor(time.Now().Add(time.Duration(tick+1) * time.Minute))
				if a.inBandMonitor == nil {
					t.Fatalf("monitor stopped at tick %d", tick)
				}
				marker := a.inBandMonitor.marker
				response := marker + "BEGIN__\r\nHOST=server\r\n" + marker + "END__:0__\r\nroot@server:~# "
				if bytewise {
					for _, b := range []byte(response) {
						a.processSSHOutput([]byte{b})
					}
				} else {
					a.processSSHOutput([]byte(response))
				}
				if a.inBandMonitor != nil || !a.atPrompt {
					t.Fatal("fresh prompt did not restore input")
				}
				screen := monitorScreenText(a)
				if !strings.Contains(screen, "HOST=server") || !strings.Contains(screen, "退出码：0") || strings.Contains(screen, marker) {
					t.Fatalf("missing visible result: %s", screen)
				}
				if tick == 0 {
					firstScreen, firstHistory = screen, len(a.terminalHistory)
				} else if screen != firstScreen || len(a.terminalHistory) != firstHistory {
					t.Fatalf("recurring monitor changed the terminal at tick %d: %s", tick, screen)
				}
			}
			if a.panels.ByID("system").RefreshCount != 20 {
				t.Fatal("silent monitoring stopped updating the panel")
			}
		})
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
