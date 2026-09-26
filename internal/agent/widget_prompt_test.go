package agent

import (
	"strings"
	"testing"
)

func TestWidgetProtocolSurvivesInitialAndContinuationPrompts(t *testing.T) {
	r := Request{Event: "start", SystemContext: map[string]string{
		"monitorWidgetV1": WidgetGuidance,
		"ordinary_log":    strings.Repeat("log-entry\n", 400),
	}}
	for name, prompt := range map[string]string{
		"initial":      buildCodexPrompt(r, nil, "create a monitor"),
		"continuation": buildCodexContinuationPrompt(r, "repair the monitor"),
	} {
		if !strings.Contains(prompt, "monitorWidgetV1="+WidgetGuidance) {
			t.Fatalf("%s prompt dropped part of the SDK contract", name)
		}
		if strings.Contains(prompt, r.SystemContext["ordinary_log"]) {
			t.Fatalf("%s prompt no longer bounds ordinary context", name)
		}
	}
}
