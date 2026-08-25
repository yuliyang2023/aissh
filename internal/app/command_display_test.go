package app

import (
	"strings"
	"testing"
)

func TestLongCommandWrapsAtShellOperators(t *testing.T) {
	command := `journalctl -u nginx --since "30 minutes ago" --no-pager | grep -Ei "error code|timeout" && systemctl status nginx --no-pager`
	display := formatCommandForDisplay(command, 42, 0)
	joined := strings.Join(display.Lines, "\n")
	for _, want := range []string{"$ journalctl", "  ↳ | grep", "  ↳ && systemctl", `"30 minutes ago"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command display missing %q: %q", want, joined)
		}
	}
	for _, line := range display.Lines {
		if textWidth(line) > 42 {
			t.Fatalf("line width=%d: %q", textWidth(line), line)
		}
	}
}

func TestLongCommandShowsExplicitFoldSummary(t *testing.T) {
	command := "curl --fail --silent --show-error https://example.invalid/really/long/path --header 'Authorization: Bearer token' | jq '.items[] | select(.healthy == false)'"
	display := formatCommandForDisplay(command, 30, 3)
	if !display.Truncated || len(display.Lines) != 3 {
		t.Fatalf("display=%#v", display)
	}
	if !strings.Contains(display.Lines[2], "[展开全部]") || !strings.Contains(display.Lines[2], "Ctrl+O") {
		t.Fatalf("fold summary=%q", display.Lines[2])
	}
}

func TestMultilineCommandPreservesIndentation(t *testing.T) {
	command := "cat <<'EOF'\n  keep-leading-spaces\nEOF"
	display := formatCommandForDisplay(command, 40, 0)
	joined := strings.Join(display.Lines, "\n")
	if !strings.Contains(joined, "  ↳   keep-leading-spaces") {
		t.Fatalf("multiline indentation was not preserved: %q", joined)
	}
}

func TestRiskDisplayUsesReadableLabels(t *testing.T) {
	for input, want := range map[string]string{
		"read_only": "只读",
		"medium":    "中风险",
		"high":      "高风险",
	} {
		if got := riskDisplay(input); got != want {
			t.Fatalf("risk %q=%q", input, got)
		}
	}
}
