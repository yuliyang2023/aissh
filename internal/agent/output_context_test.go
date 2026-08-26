package agent

import (
	"strings"
	"testing"
)

func TestLimitOutputContextKeepsHeadTailAndMarksOmission(t *testing.T) {
	value := "HEADER\n" + strings.Repeat("middle", 100) + "\nFINAL ERROR"
	limited := LimitOutputContext(value, 90)
	for _, want := range []string{"HEADER", "FINAL ERROR", "aissh omitted"} {
		if !strings.Contains(limited, want) {
			t.Fatalf("limited output missing %q: %q", want, limited)
		}
	}
	if len([]rune(limited)) >= len([]rune(value)) {
		t.Fatalf("output was not limited: limited=%d original=%d", len([]rune(limited)), len([]rune(value)))
	}
	if len([]rune(limited)) > 90 || LimitOutputContext(limited, 90) != limited {
		t.Fatalf("limit was not exact or idempotent: %d characters", len([]rune(limited)))
	}
}

func TestProviderOutputContextLimitIsConfigurable(t *testing.T) {
	provider := NewDirectModelProvider("Model", "https://model.example", "", "test", false)
	provider.OutputContextChars = 50000
	value := "BEGIN\n" + strings.Repeat("x", 30000) + "\nEND"
	request := provider.requestText(&codexSession{Proposals: make(map[string]Proposal)}, Request{Prompt: "分析", RecentOutput: value})
	if !strings.Contains(request, "BEGIN") || !strings.Contains(request, "END") || strings.Contains(request, "aissh omitted") {
		t.Fatalf("provider unexpectedly truncated configured context: length=%d", len([]rune(request)))
	}
}
