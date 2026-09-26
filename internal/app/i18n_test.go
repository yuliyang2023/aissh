package app

import (
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestEnglishUIUsesLocalizedPanelsConversationAndApproval(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Language: "en", Version: "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if application.panels.ByID("system").Title != "System" || application.panels.ByID("report") != nil {
		t.Fatalf("panel titles: %#v", application.panels.Panels())
	}
	application.atPrompt = true
	for label, output := range map[string]string{
		"monitor header": application.monitorHeaderLine(70),
		"panel menu":     strings.Join(application.panelMenuLines(), "\n"),
		"AI header":      application.aiHeaderLine(50),
		"status bar":     application.statusLine(100),
	} {
		for _, want := range map[string][]string{
			"monitor header": {"Monitor", "Layout:Auto"},
			"panel menu":     {"Add panel", "System overview", "Custom read-only command"},
			"AI header":      {"AI Assistant", "Ready"},
			"status bar":     {"Selection mode", "Ctrl+] Enable scrolling", "/ai AI chat", "aissh v1.2.3"},
		}[label] {
			if !strings.Contains(output, want) {
				t.Fatalf("%s missing %q: %q", label, want, output)
			}
		}
	}

	application.mainCols = 80
	application.contentRows = 22
	application.rows = 24
	application.proposal = &agent.Proposal{Command: "uptime", Risk: "read_only"}
	approval := application.renderApprovalCard()
	for _, want := range []string{"AI command confirmation", "Read-only", "[ yes Run ]", "[ no Reject ]", "Enter yes/no"} {
		if !strings.Contains(approval, want) {
			t.Fatalf("English approval missing %q: %q", want, approval)
		}
	}
	for risk, label := range map[string]string{"read-only": "Read-only", "critical": "Critical risk", "unexpected": "Unknown risk"} {
		application.proposal.Risk = risk
		if approval := application.renderApprovalCard(); !strings.Contains(approval, label) {
			t.Fatalf("English approval missing %q: %q", label, approval)
		}
		if content := application.proposalConversationContent(*application.proposal); !strings.Contains(content, "Risk: "+label) {
			t.Fatalf("English conversation missing risk %q: %q", label, content)
		}
	}
}

func TestEnglishLongCommandSummaryIsLocalized(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	display := application.formatCommandForDisplay("journalctl -u nginx --since today --no-pager | grep error | tail -n 100", 24, 2)
	joined := strings.Join(display.Lines, "\n")
	if !strings.Contains(joined, "Expand all") || strings.Contains(joined, "展开全部") {
		t.Fatalf("command summary=%q", joined)
	}
}

func TestStatusLineShowsUsefulShortcutsAndRightAlignedVersion(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Version: "v1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if line := application.statusLine(80); strings.Contains(line, "Ctrl+") || strings.Contains(line, "/ai") {
		t.Fatalf("status line shows unavailable shortcuts away from a shell prompt: %q", line)
	}
	application.atPrompt = true
	line := application.statusLine(80)
	for _, want := range []string{"选择模式", "Ctrl+] 开启滚动", "/ai AI 对话"} {
		if !strings.Contains(line, want) {
			t.Fatalf("status line missing %q: %q", want, line)
		}
	}
	if strings.Contains(line, "原生选择") {
		t.Fatalf("status line contains non-actionable hint: %q", line)
	}
	if !strings.HasSuffix(line, "aissh v1.2.3") {
		t.Fatalf("version is not right aligned: %q", line)
	}
	if textWidth(line) != 80 {
		t.Fatalf("status line width=%d, want 80: %q", textWidth(line), line)
	}
}
