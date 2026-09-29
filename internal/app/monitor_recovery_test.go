package app

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/history"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
)

func TestMonitorRefreshResumesPausedCollectionAfterFreshPrompt(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.config.DesktopUI = true
	a.screenOut = io.Discard
	a.scheduleMonitors()
	a.checkInBandMonitorTimeout(time.Now().Add(time.Hour))
	a.processSSHOutput([]byte("\r\nroot@server:~# "))
	terminal.Reset()
	a.scheduleMonitors()
	if terminal.Len() != 0 {
		t.Fatal("timed-out collection restarted without user action")
	}
	if err := a.handleDesktopCommand(credentialui.WorkbenchCommand{Kind: "monitor-refresh", PanelID: "system", Deadline: time.Now().Add(time.Minute).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	a.scheduleMonitors()
	if a.monitorPrimaryPaused || a.inBandMonitor == nil || terminal.Len() == 0 {
		t.Fatal("explicit refresh left monitoring permanently paused")
	}
	if strings.Contains(terminal.String(), "\x03") {
		t.Fatal("refresh injected an interrupt")
	}
}

func TestMonitorRefreshCannotResumeBeforeShellIsSafe(t *testing.T) {
	for _, mode := range []string{"awaiting prompt", "typing", "execution", "sensitive"} {
		t.Run(mode, func(t *testing.T) {
			a, terminal := newMonitorTestApp(t)
			a.config.DesktopUI = true
			a.monitorPrimaryPaused = true
			switch mode {
			case "awaiting prompt":
				a.atPrompt = false
				a.monitorAwaitingPrompt = true
			case "typing":
				a.lineInput = []byte("unfinished")
			case "execution":
				a.execution = &executionState{}
			case "sensitive":
				a.sensitive = true
			}
			err := a.handleDesktopCommand(credentialui.WorkbenchCommand{Kind: "monitor-refresh", PanelID: "system", Deadline: time.Now().Add(time.Minute).UnixMilli()})
			if err == nil || !a.monitorPrimaryPaused || terminal.Len() != 0 {
				t.Fatalf("unsafe resume accepted: %v", err)
			}
		})
	}
}

func TestPeriodicPipelineWaitDoesNotFabricateFailedSamples(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.config.DesktopUI = true
	p := a.panels.ByID("system")
	p.Pipeline = &monitorplan.Definition{Sources: []monitorplan.Source{{ID: "system", Command: panel.BuiltinOverviewCommand, Required: true, TimeoutMS: 8000}}}
	p.LastUpdated = time.Now()
	a.atPrompt = false
	a.monitorAwaitingPrompt = true
	before := len(a.chat)
	for i := 1; i <= 6; i++ {
		a.schedulePipelines(time.Now().Add(time.Duration(i) * time.Hour))
	}
	if len(a.pipelineBatches) != 0 || len(a.chat) != before || p.RefreshCount != 0 || terminal.Len() != 0 {
		t.Fatal("waiting for the shell generated failed samples or repeated notices")
	}
	a.processSSHOutput([]byte("\r\nroot@server:~# "))
	a.schedulePipelines(time.Now())
	if a.inBandMonitor == nil || len(a.pipelineBatches) != 1 {
		t.Fatal("collection did not resume on a fresh prompt")
	}
}

func TestSSHExitRetainsDiagnosticDuringSilentMonitor(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	a.scheduleMonitors()
	if a.inBandMonitor == nil {
		t.Fatal("monitor did not start")
	}
	a.handleEvent(outputEvent{data: []byte("\r\nclient_loop: send disconnect: Broken pipe\r\n")})
	a.handleEvent(sshExitEvent{err: errors.New("exit status 255")})
	message := a.chat[len(a.chat)-1].Content
	if !strings.Contains(message, "Broken pipe") || !strings.Contains(message, "255") {
		t.Fatalf("lost SSH exit diagnostic: %q", message)
	}
}

func TestSSHExitDiagnosticIsRedactedAndPersistedOnce(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	a.config.SecretFilter = redact.New("private-token")
	a.historyStore = history.New(t.TempDir())
	a.historyTarget = "server"
	a.handleEvent(outputEvent{data: []byte("\x1b[31mclient_loop: send disconnect: Broken pipe private-token\x1b[0m\r\n")})
	a.handleEvent(sshExitEvent{err: errors.New("exit status 255")})
	a.handleEvent(sshExitEvent{err: errors.New("exit status 255")})
	messages, err := a.historyStore.Load("server")
	if err != nil || len(messages) != 1 {
		t.Fatalf("exit history: %v, %v", messages, err)
	}
	for _, text := range []string{messages[0].Content, a.chat[len(a.chat)-1].Content} {
		if !strings.Contains(text, "Broken pipe") || strings.Contains(text, "private-token") || strings.Contains(text, "\x1b") {
			t.Fatalf("unsafe or missing diagnostic: %q", text)
		}
	}
}
