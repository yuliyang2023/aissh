package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

func TestMonitorRequiresManualAuditEvenWithAlwaysYesAndExecTransport(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.config.DesktopUI, a.config.AlwaysYes = true, true
	a.monitorViaPrimary = false
	a.monitorDecisions = nil
	var visible bytes.Buffer
	a.screenOut = &visible
	p := a.panels.ByID("system")
	p.Command = "printf 'audit sample\\n'"
	a.scheduleMonitors()
	if terminal.Len() != 0 || a.inBandMonitor != nil || a.monitorReview == nil || a.desktopState().Proposal == nil {
		t.Fatal("unapproved monitor executed or bypassed command review")
	}
	if a.proposal.Command != p.Command || !strings.Contains(a.proposal.Explanation, "重复采集") {
		t.Fatal("review did not disclose exact script and recurring scope")
	}
	a.approveProposal() // Must be handled locally, without an AI approval request.
	a.scheduleMonitors()
	if a.inBandMonitor == nil || !strings.Contains(terminal.String(), "audit sample") {
		t.Fatal("approved collector did not use primary terminal")
	}
	marker := a.inBandMonitor.marker
	a.processSSHOutput([]byte(marker + "BEGIN__\r\naudit sample\r\n" + marker + "END__:0__\r\nroot@server:~# "))
	for _, want := range []string{p.Command, "audit sample", "退出码：0"} {
		if !strings.Contains(visible.String(), want) {
			t.Fatalf("terminal omitted %q: %q", want, visible.String())
		}
	}
	if strings.Contains(visible.String(), marker) || strings.Contains(a.recentOutput.ContextString(), "audit sample") {
		t.Fatal("private framing leaked or collector output became implicit AI context")
	}
	a.monitorLast[p.ID] = time.Time{}
	a.scheduleMonitors()
	if a.proposal != nil || a.inBandMonitor == nil {
		t.Fatal("unchanged recurring collector required another approval")
	}
}

func TestMonitorRejectChangeStaleDecisionAndExplicitRetry(t *testing.T) {
	for _, change := range []string{"command", "interval", "AI access", "binding"} {
		t.Run(change, func(t *testing.T) {
			a, terminal := newMonitorTestApp(t)
			p := a.panels.ByID("system")
			switch change {
			case "command":
				p.Command = "printf changed"
			case "interval":
				p.Interval /= 2
			case "AI access":
				p.AllowAIRead = !p.AllowAIRead
			case "binding":
				p.BindingID = "another instance"
			}
			a.scheduleMonitors()
			if a.monitorReview == nil || terminal.Len() != 0 {
				t.Fatal("changed collector reused old approval")
			}
			a.rejectProposal()
			for i := 0; i < 3; i++ {
				a.scheduleMonitors()
			}
			if a.proposal != nil || terminal.Len() != 0 || a.inBandMonitor != nil {
				t.Fatal("rejected collector was retried")
			}
			a.config.DesktopUI = true
			if err := a.handleDesktopCommand(credentialui.WorkbenchCommand{Kind: "monitor-refresh", PanelID: p.ID, Deadline: time.Now().Add(time.Minute).UnixMilli()}); err != nil {
				t.Fatal(err)
			}
			a.scheduleMonitors()
			if a.monitorReview == nil {
				t.Fatal("explicit refresh did not allow a new review")
			}
			p.Command = "printf changed_again"
			a.approveProposal()
			a.scheduleMonitors()
			if terminal.Len() != 0 || a.monitorReview == nil || a.proposal.Command != p.Command {
				t.Fatal("stale approval executed modified command")
			}
		})
	}
}

func TestPipelineProbeNeedsReviewBeforeAnySourceAndRejectStopsBatch(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.config.DesktopUI = true
	a.screenOut = io.Discard
	a.agent = nil
	sources := []monitorplan.Source{
		{ID: "first", Command: "printf first", TimeoutMS: 1000, Required: true},
		{ID: "second", Command: "printf second", TimeoutMS: 1000, Required: true},
	}
	_, _, err := a.executePanelAction(agent.PanelAction{Action: "probe", Panel: agent.PanelSpec{ID: "audited", Pipeline: &monitorplan.Request{ProtocolVersion: 2, Sources: sources}}})
	if err != nil {
		t.Fatal(err)
	}
	a.schedulePipelines(time.Now())
	if terminal.Len() != 0 || a.monitorReview == nil || a.proposal.Command != sources[0].Command {
		t.Fatal("probe bypassed first-source review")
	}
	a.schedulePipelines(time.Now().Add(10 * time.Minute))
	if len(a.pipelineBatches) != 1 || a.pipelineDrafts["audited"] == nil {
		t.Fatal("waiting for human review expired the draft")
	}
	a.approveProposal()
	a.schedulePipelines(time.Now())
	if a.inBandMonitor == nil {
		t.Fatal("approved probe did not run")
	}
	marker := a.inBandMonitor.marker
	a.processSSHOutput([]byte(marker + "BEGIN__\r\nfirst\r\n" + marker + "END__:0__\r\nroot@server:~# "))
	terminal.Reset()
	a.schedulePipelines(time.Now())
	if terminal.Len() != 0 || a.monitorReview == nil || a.proposal.Command != sources[1].Command {
		t.Fatal("one source approval authorized another script")
	}
	a.rejectProposal()
	if len(a.pipelineBatches) != 0 || a.panels.ByID("audited") != nil || terminal.Len() != 0 {
		t.Fatal("rejected probe remained active")
	}
}

func TestMonitorStreamsShortOutputAndFailureWithoutAcceptingFakePrompt(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	a.config.DesktopUI = true
	var visible bytes.Buffer
	a.screenOut = &visible
	a.scheduleMonitors()
	marker := a.inBandMonitor.marker
	a.processSSHOutput([]byte(marker + "BEGIN__\r\npermission denied\r\nroot@server:~# "))
	if !strings.Contains(visible.String(), "permission denied") || a.atPrompt {
		t.Fatal("short output was hidden or collector forged an idle prompt")
	}
	a.processSSHOutput([]byte("\r\n" + marker + "END__:7__"))
	if a.inBandMonitor != nil || a.atPrompt || !strings.Contains(visible.String(), "退出码：7") {
		t.Fatal("failure was hidden or input released before the real prompt")
	}
}

func TestVisibleMonitorFramingAcrossEverySplit(t *testing.T) {
	const marker = "__AUDIT_TEST__"
	const content = "a short sample\r\n"
	response := marker + "BEGIN__\r\n" + content + "\r\n" + marker + "END__:0__\r\nhost$ "
	for split := 1; split < len(response); split++ {
		state := &inBandMonitorState{visible: true, marker: marker}
		left, done, _, _ := state.consume([]byte(response[:split]))
		if done {
			continue // The full completion marker arrived in the first chunk.
		}
		right, done, sample, err := state.consume([]byte(response[split:]))
		visible := string(append(left, right...))
		if !done || err != nil || sample != content || !strings.Contains(visible, content) || strings.Contains(visible, marker) || strings.Count(visible, "a short sample") != 1 {
			t.Fatalf("split=%d visible=%q sample=%q err=%v", split, visible, sample, err)
		}
	}
}

func TestVisibleMonitorDoesNotReplayConPTYCursorMovesOverAudit(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	a.config.DesktopUI = true
	var visible bytes.Buffer
	a.screenOut = &visible
	a.scheduleMonitors()
	marker := a.inBandMonitor.marker
	response := marker + "BEGIN__\x1b[3;1Hfirst\x1b[4;1Hsecond\x1b[5;1H" + marker + "END__:0__\x1b[2;1Hroot@server:~# "
	for _, b := range []byte(response) {
		a.processSSHOutput([]byte{b})
	}
	if !a.atPrompt || !strings.Contains(visible.String(), "first\r\nsecond") || strings.Contains(visible.String(), "[2;1H") || strings.Contains(visible.String(), "[3;1H") {
		t.Fatalf("cursor repaint lost the audit transcript: %q", visible.String())
	}
}

func TestMonitorApprovalCannotStartAnotherCollectorInSameTick(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	p := a.panels.Open(panel.Panel{ID: "second", Kind: panel.KindMonitor, Command: "printf second", Interval: time.Second})
	a.ensureMonitorAudit(a.panelMonitorAudit(p))
	a.approveProposal()
	a.resetMonitorAudit("system", false)
	a.scheduleMonitors()
	if a.proposal == nil || terminal.Len() != 0 || a.inBandMonitor != nil {
		t.Fatal("another approved collector started while review was pending")
	}
}
