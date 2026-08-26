package app

import (
	"strings"
	"testing"
)

func TestBeginCommandOutputResetsPreviousCommandAndKeepsCommand(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, AIContextChars: 30000})
	if err != nil {
		t.Fatal(err)
	}
	application.recentOutput.Append("old command output")
	application.beginCommandOutput("journalctl -u nginx")
	context := application.recentOutput.ContextString()
	if strings.Contains(context, "old command") || !strings.Contains(context, "$ journalctl -u nginx") {
		t.Fatalf("command context=%q", context)
	}
	if application.recentOutput.limit != 30000 {
		t.Fatalf("capture characters=%d, want 30000", application.recentOutput.limit)
	}
}

func TestContextBufferKeepsActualBeginningAndEnd(t *testing.T) {
	buffer := newContextBuffer(120)
	buffer.Append("HEADER\n")
	buffer.Append(strings.Repeat("middle", 200))
	buffer.Append("\nFINAL ERROR")
	context := buffer.ContextString()
	for _, want := range []string{"HEADER", "FINAL ERROR", "aissh omitted"} {
		if !strings.Contains(context, want) {
			t.Fatalf("context missing %q: %q", want, context)
		}
	}
	if len([]rune(context)) > 120 {
		t.Fatalf("context length=%d, want <=120", len([]rune(context)))
	}
}
