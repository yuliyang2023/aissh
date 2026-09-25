package app

import (
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestSidebarCommandsAtSSHPrompt(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	initialWidth := a.mainCols
	for _, step := range []struct {
		command   string
		collapsed bool
	}{
		{"/sidebar", true},
		{"/sidebar", false},
		{"/sidebar hide", true},
		{"/sidebar hide", true},
		{"/SIDEBAR SHOW", false},
		{"/sidebar show", false},
		{"/sidebar toggle", true},
		{"  /sidebar show  ", false},
	} {
		terminal.Reset()
		a.dirty = false
		// Exercise incremental typing and backspace through the input router.
		for _, value := range []byte(step.command + "x\x7f\r") {
			a.handleInput([]byte{value})
		}
		if a.sidebarCollapsed != step.collapsed || a.focus != focusTerminal || !a.atPrompt || !a.dirty {
			t.Fatalf("%q: collapsed=%v focus=%v prompt=%v dirty=%v", step.command, a.sidebarCollapsed, a.focus, a.atPrompt, a.dirty)
		}
		wantWidth := initialWidth
		if step.collapsed {
			wantWidth = a.cols - 1
		}
		if cols, _ := a.virtual.Size(); a.mainCols != wantWidth || cols != wantWidth {
			t.Fatalf("%q: terminal widths=%d/%d want=%d", step.command, a.mainCols, cols, wantWidth)
		}
		if got, want := terminal.String(), step.command+"x\x7f\x15"; got != want {
			t.Fatalf("%q: SSH input=%q want=%q", step.command, got, want)
		}
		if len(a.lineInput) != 0 || a.lineAnchored || a.aiLineCapture || a.agentBusy || a.lastCommand != "" {
			t.Fatalf("%q left pending input or started a remote/AI command", step.command)
		}
	}
	terminal.Reset()
	a.handleInput([]byte("printf ready\r"))
	if got := terminal.String(); got != "printf ready\r" || a.atPrompt {
		t.Fatalf("normal SSH command after toggling=%q prompt=%v", got, a.atPrompt)
	}
}

func TestPastedSidebarCommandWaitsForEnter(t *testing.T) {
	for _, prefix := range []string{"", "/side"} {
		for _, suffix := range []string{"", "\r\n"} {
			t.Run(prefix+suffix, func(t *testing.T) {
				a, terminal := newMonitorTestApp(t)
				a.handleInput([]byte(prefix))
				payload := strings.TrimPrefix("/sidebar hide", prefix) + suffix
				a.handleInput(append(append([]byte(nil), terminalPasteStart...), []byte(payload)...))
				a.handleInput(terminalPasteEnd)
				if a.sidebarCollapsed || !a.aiLineCapture || string(a.lineInput) != "/sidebar hide" {
					t.Fatalf("paste executed early or lost input: collapsed=%v line=%q", a.sidebarCollapsed, a.lineInput)
				}
				want := ""
				if prefix != "" {
					want = prefix + "\x15"
				}
				a.handleInput([]byte{'\r'})
				if !a.sidebarCollapsed || terminal.String() != want || !a.atPrompt {
					t.Fatalf("pasted command: collapsed=%v SSH=%q prompt=%v", a.sidebarCollapsed, terminal.String(), a.atPrompt)
				}
			})
		}
	}
}

func TestSidebarCommandDoesNotInterceptRemoteInput(t *testing.T) {
	for _, mode := range []string{"running", "password", "untracked", "path", "shell escape", "multiline paste"} {
		t.Run(mode, func(t *testing.T) {
			a, terminal := newMonitorTestApp(t)
			input, want := "/sidebar\r", "/sidebar\r"
			switch mode {
			case "running":
				a.handleInput([]byte("sleep 60\r"))
				a.processSSHOutput([]byte("\r\nrunning..."))
				terminal.Reset()
			case "password":
				a.sensitive = true
			case "untracked":
				a.lineReliable = false
			case "path":
				input, want = "/sidebar/tool\r", "/sidebar/tool\r"
			case "shell escape":
				input, want = ":shell /sidebar\r", ":shell /sidebar\x15/sidebar\r"
			case "multiline paste":
				input = string(terminalPasteStart) + "/sidebar\nprintf ready" + string(terminalPasteEnd)
				want = input
			}
			a.handleInput([]byte(input))
			if a.sidebarCollapsed || terminal.String() != want || a.inlineAI != nil {
				t.Fatalf("remote input intercepted: collapsed=%v SSH=%q want=%q", a.sidebarCollapsed, terminal.String(), want)
			}
		})
	}
}

func TestSidebarCommandExplainsInvalidArgumentsAndCompactLayout(t *testing.T) {
	for _, mode := range []string{"invalid", "compact"} {
		t.Run(mode, func(t *testing.T) {
			a, terminal := newMonitorTestApp(t)
			command, want := "/sidebar unknown", "用法：/sidebar"
			if mode == "compact" {
				a.setViewportSize(80, 30)
				command, want = "/sidebar show", "92 列"
			}
			a.handleInput([]byte(command + "\r"))
			if a.sidebarCollapsed || a.inlineAI == nil || !strings.Contains(a.inlineAI.Response, want) || a.agentBusy {
				t.Fatalf("missing local guidance: collapsed=%v inline=%#v busy=%v", a.sidebarCollapsed, a.inlineAI, a.agentBusy)
			}
			if terminal.String() != command+"\x15" || !a.atPrompt {
				t.Fatalf("command reached remote shell: %q prompt=%v", terminal.String(), a.atPrompt)
			}
		})
	}
}

func TestSidebarCommandInAIEditorPreservesApproval(t *testing.T) {
	a, terminal := newMonitorTestApp(t)
	a.proposal = &agent.Proposal{Command: "uptime", Risk: "read_only"}
	a.openAIConversation()
	a.handleInput([]byte("/sidebar hide\r"))
	if !a.sidebarCollapsed || a.focus != focusApproval || a.proposal == nil || len(a.editor) != 0 || a.agentBusy || terminal.Len() != 0 {
		t.Fatalf("hidden approval lost focus or command escaped: collapsed=%v focus=%v SSH=%q", a.sidebarCollapsed, a.focus, terminal.String())
	}
}
