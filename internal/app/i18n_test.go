package app

import (
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestEnglishUIUsesLocalizedPanelsConversationAndApproval(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if application.panels.ByID("system").Title != "System" || application.panels.ByID("report") != nil {
		t.Fatalf("panel titles: %#v", application.panels.Panels())
	}
	for label, output := range map[string]string{
		"monitor header": application.monitorHeaderLine(70),
		"panel menu":     strings.Join(application.panelMenuLines(), "\n"),
		"AI header":      application.aiHeaderLine(50),
		"status bar":     application.inputStatus(),
	} {
		for _, want := range map[string][]string{
			"monitor header": {"Monitor", "Layout:Auto"},
			"panel menu":     {"Add panel", "System overview", "Custom read-only command"},
			"AI header":      {"AI Assistant", "Ready"},
			"status bar":     {"Click AI conversation", "Use /ai to ask"},
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
