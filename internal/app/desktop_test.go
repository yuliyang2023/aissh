package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
	"github.com/hinshun/vt10x"
)

func desktopTestApp(t *testing.T) *App {
	t.Helper()
	a, err := New(Config{DesktopUI: true, SSHArgs: []string{"server"}, SecretFilter: redact.New("super-secret")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.cancel)
	a.agent = approvalTestProvider{}
	a.screenOut = io.Discard
	a.setViewportSize(120, 36)
	a.virtual = vt10x.New(vt10x.WithSize(a.mainCols, a.contentRows))
	return a
}

func TestDesktopKeepsTerminalFreeOfSidebarAndApproval(t *testing.T) {
	a := desktopTestApp(t)
	a.focus = focusAI
	a.setViewportSize(140, 36)
	a.toggleSidebar()
	a.ensureSidebarVisible()
	a.handleLocalSidebarCommand("/sidebar show", true)
	if a.sidebarVisible() || a.mainCols != 140 || a.contentRows != 36 || a.sidebarToggleButton() != "" {
		t.Fatal("desktop exposed terminal sidebar")
	}
	a.finishAgentTurn(agent.Turn{Proposal: &agent.Proposal{ID: "p", Command: "secret-command-card", Risk: "high"}}, nil)
	var output bytes.Buffer
	a.screenOut = &output
	a.render()
	if output.Len() != 0 {
		t.Fatalf("CLI rendering leaked into the desktop terminal: %q", output.String())
	}
	if a.desktopState().Proposal == nil {
		t.Fatal("client lost approval")
	}
	a.setViewportSize(55, 18)
	a.focus = focusAI
	output.Reset()
	a.render()
	if strings.Contains(output.String(), "secret-command-card") || a.focus != focusTerminal {
		t.Fatal("compact desktop replaced terminal with AI")
	}
}

func TestDesktopPromptIsStructuredAndProposalCannotBeOverwritten(t *testing.T) {
	a := desktopTestApp(t)
	command := credentialui.WorkbenchCommand{Kind: "prompt", Text: "请检查内存\n不要重启", Deadline: time.Now().Add(time.Second).UnixMilli()}
	if err := a.handleDesktopCommand(command); err != nil {
		t.Fatal(err)
	}
	if a.inlineAI != nil || !a.agentBusy || a.chat[len(a.chat)-1].Content != command.Text {
		t.Fatal("prompt did not reach native AI conversation")
	}
	select {
	case event := <-a.events:
		a.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("AI did not finish")
	}
	a.proposal = &agent.Proposal{ID: "pending", Command: "uptime"}
	if err := a.handleDesktopCommand(command); err == nil || a.proposal == nil {
		t.Fatal("new prompt overwrote pending command")
	}
	a.startInlineAI("terminal question", "terminal question", true)
	if a.proposal == nil || a.proposal.ID != "pending" {
		t.Fatal("terminal AI shortcut overwrote pending command")
	}
	command.Deadline = time.Now().Add(-time.Second).UnixMilli()
	if err := a.handleDesktopCommand(command); err == nil {
		t.Fatal("expired prompt accepted")
	}
}

func TestDesktopApprovalChecksIdentityAndIdleShellTwice(t *testing.T) {
	a := desktopTestApp(t)
	a.atPrompt = true
	a.proposal = &agent.Proposal{ID: "current", Command: "uptime", Risk: "low"}
	command := credentialui.WorkbenchCommand{Kind: "approve", ProposalID: "old", Deadline: time.Now().Add(time.Second).UnixMilli()}
	if err := a.handleDesktopCommand(command); err == nil {
		t.Fatal("stale approval accepted")
	}
	command.ProposalID = "current"
	a.lineInput = []byte("unfinished")
	if err := a.handleDesktopCommand(command); err == nil {
		t.Fatal("approved into partial shell input")
	}
	a.lineInput = nil
	if err := a.handleDesktopCommand(command); err != nil {
		t.Fatal(err)
	}
	// Input can arrive during the asynchronous provider approval round trip.
	a.lineInput = []byte("typed-during-approval")
	select {
	case event := <-a.events:
		a.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("missing approval response")
	}
	if a.execution != nil || a.proposal == nil || a.approvalStatus == "" {
		t.Fatal("command executed after shell state changed")
	}
}

func TestDesktopStateRedactsAndCopiesData(t *testing.T) {
	a := desktopTestApp(t)
	a.chat = append(a.chat, chatMessage{Role: "assistant", Content: "super-secret", Time: time.Now()})
	a.proposal = &agent.Proposal{ID: "p", Command: "echo super-secret", Explanation: "super-secret"}
	a.panels.UpdateContent("system", "HOST=super-secret", "live", false)
	a.panels.ByID("system").Command = "printf super-secret"
	a.monitorActive["system"] = true
	snapshot := a.desktopState()
	if strings.Contains(snapshot.Messages[len(snapshot.Messages)-1].Content, "super-secret") || strings.Contains(snapshot.Proposal.Command, "super-secret") || strings.Contains(snapshot.Panels[0].Content, "super-secret") {
		t.Fatal("state contains credential")
	}
	if strings.Contains(snapshot.Panels[0].Command, "super-secret") || snapshot.Panels[0].Command == "" || snapshot.Panels[0].Origin != "system" || !snapshot.Panels[0].Collecting {
		t.Fatal("inventory metadata omitted or leaked an unredacted command")
	}
	a.proposal.Command = "changed"
	a.panels.UpdateContent("system", "changed", "live", false)
	a.panels.ByID("system").Command = "changed"
	if snapshot.Proposal.Command == "changed" || snapshot.Panels[0].Content == "changed" || snapshot.Panels[0].Command == "changed" {
		t.Fatal("snapshot shares mutable app state")
	}
}

func TestDesktopMonitorPolicyAndRefresh(t *testing.T) {
	a := desktopTestApp(t)
	command := credentialui.WorkbenchCommand{Kind: "monitor-open", PanelID: "custom", Text: "rm -rf /tmp/data", Deadline: time.Now().Add(time.Second).UnixMilli()}
	if err := a.handleDesktopCommand(command); err == nil {
		t.Fatal("destructive monitor accepted")
	}
	command.PanelID = "processes"
	if err := a.handleDesktopCommand(command); err != nil {
		t.Fatal(err)
	}
	a.monitorLast["desktop-processes"] = time.Now()
	command.Kind, command.PanelID = "monitor-refresh", "desktop-processes"
	if err := a.handleDesktopCommand(command); err != nil || !a.monitorLast[command.PanelID].IsZero() {
		t.Fatalf("refresh: %v", err)
	}
	command.Kind, command.PanelID = "monitor-close", "system"
	if err := a.handleDesktopCommand(command); err == nil {
		t.Fatal("closed pinned system panel")
	}
	command.PanelID = "desktop-processes"
	if err := a.handleDesktopCommand(command); err != nil {
		t.Fatal(err)
	}
}
