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

func TestCommandDisplayKeepsFileDescriptorRedirectsTogether(t *testing.T) {
	command := `if ! command -v firewall-cmd >/dev/null 2>&1 || ! firewall-cmd --state >/dev/null 2>&1; then echo failed >&2; fi`
	display := formatCommandForDisplay(command, 46, 0)
	joined := strings.Join(display.Lines, "\n")
	for _, want := range []string{"2>&1", ">&2"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("command display split redirect %q: %q", want, joined)
		}
	}
	if strings.Contains(joined, "↳ & 1") || strings.Contains(joined, "↳ & 2") {
		t.Fatalf("command display rendered redirect ampersand as a chain operator: %q", joined)
	}
}

func TestShellDisplayTokensDistinguishesAmpersandRedirectsAndOperators(t *testing.T) {
	command := `printf x 2>&1 |& tee log &>/dev/null & wait`
	want := []string{"printf", "x", "2>&1", "|&", "tee", "log", "&>/dev/null", "&", "wait"}
	got := shellDisplayTokens(command)
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tokens=%q, want %q", got, want)
	}
}

func TestRiskDisplayUsesReadableLabels(t *testing.T) {
	for input, want := range map[string]string{
		"read_only":   "只读",
		"read-only":   "只读",
		"readonly":    "只读",
		"safe":        "只读",
		" LOW ":       "低风险",
		"medium":      "中风险",
		"high":        "高风险",
		"dangerous":   "高风险",
		"destructive": "高风险",
		"critical":    "极高风险",
		"":            "风险未知",
		"unexpected":  "风险未知",
	} {
		if got := riskDisplay(input); got != want {
			t.Fatalf("risk %q=%q", input, got)
		}
	}
}
