package monitor

import (
	"errors"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

func TestIsAuthenticationErrorRequiresSSHConnectionFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "openssh password authentication",
			err:  &RunError{Message: "Permission denied", ExitCode: 255},
			want: true,
		},
		{
			name: "remote command permission denied",
			err:  &RunError{Message: "cat: /root/file: Permission denied", ExitCode: 1},
		},
		{
			name: "unrelated connection failure",
			err:  &RunError{Message: "Connection timed out", ExitCode: 255},
		},
		{
			name: "plain monitor error",
			err:  errors.New("permission denied"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsAuthenticationError(test.err); got != test.want {
				t.Fatalf("IsAuthenticationError(%v)=%v want=%v", test.err, got, test.want)
			}
		})
	}
}

func TestBuiltinOverviewLabelsUseSelectedLanguage(t *testing.T) {
	output := "HOST=server\nUPTIME=1 day\nLOAD=0.1\nMEM=1/2 GB\nDISK=1/10 GB\nPROCS=20\n"
	english := formatBuiltin(panel.BuiltinOverviewCommand, output, "en")
	for _, want := range []string{"Host", "Uptime", "Load", "Memory", "Root disk", "Processes"} {
		if !strings.Contains(english, want) {
			t.Fatalf("English overview missing %q: %q", want, english)
		}
	}
	chinese := formatBuiltin(panel.BuiltinOverviewCommand, output, "zh-CN")
	if !strings.Contains(chinese, "主机") || !strings.Contains(chinese, "根分区") {
		t.Fatalf("Chinese overview=%q", chinese)
	}
}

func TestBuiltinOverviewValuesAlignInCells(t *testing.T) {
	values := []string{"server", "1 day", "0.1", "1/2 GB", "1/10 GB", "20"}
	output := "HOST=server\nUPTIME=1 day\nLOAD=0.1\nMEM=1/2 GB\nDISK=1/10 GB\nPROCS=20\n"
	cellWidth := func(s string) int {
		cells := 0
		for _, r := range s {
			cells += terminalview.CellWidth(r)
		}
		return cells
	}
	for _, language := range []string{"en", "zh-CN"} {
		lines := strings.Split(formatBuiltin(panel.BuiltinOverviewCommand, output, language), "\n")
		if len(lines) != len(values) {
			t.Fatalf("language %s: got %d lines, want %d", language, len(lines), len(values))
		}
		column := -1
		for i, line := range lines {
			if !strings.HasSuffix(line, values[i]) {
				t.Fatalf("language %s: line %q does not end with value %q", language, line, values[i])
			}
			start := cellWidth(line) - cellWidth(values[i])
			if column == -1 {
				column = start
			} else if start != column {
				t.Fatalf("language %s: value column %d != %d in line %q", language, start, column, line)
			}
		}
	}
}
