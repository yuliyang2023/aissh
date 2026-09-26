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

func TestPipelineInstructionsReceiptsAndStrictSchemaAreComplete(t *testing.T) {
	receipt := "captureId=first-source\n" + strings.Repeat("raw sample\n", 3000) + "mapId=verified"
	if promptContextValue("monitor_pipeline_receipt", receipt) != receipt {
		t.Fatal("native receipts truncated")
	}
	if promptContextValue("monitorWidgetV2", WidgetV2Guidance) != WidgetV2Guidance {
		t.Fatal("v2 protocol truncated")
	}
	for _, key := range []string{"probe", "map", "pipeline", "protocolVersion", "dataPlan", "captureId", "mapId", "bindings"} {
		if !strings.Contains(codexResponseSchema, "\""+key+"\"") {
			t.Fatalf("schema omitted %s", key)
		}
	}
	if strings.Contains(WidgetV2Guidance, "value={data?.connections ?? null}") {
		t.Fatal("v1 scalar example leaked into v2 instructions")
	}
}
