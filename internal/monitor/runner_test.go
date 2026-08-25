package monitor

import (
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

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
