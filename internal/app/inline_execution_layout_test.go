package app

import (
	"testing"

	"github.com/hinshun/vt10x"
)

func TestInlineExecutionReviewAlignsWithPromptInLaterRead(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 64
	application.contentRows = 12
	application.rows = 14
	application.virtual = vt10x.New(vt10x.WithSize(64, 12))
	provider := executionAnalysisProvider{started: make(chan struct{}), release: make(chan struct{})}
	application.agent = provider
	defer close(provider.release)
	application.execution = &executionState{Command: "printf done", Marker: "test", Output: newContextBuffer(4096), Inline: true}
	application.processRegularSSHOutput([]byte("done\r\n" + executionMarkerPrefix("test") + "0\a"))
	state := application.inlineAI
	if state == nil || !state.AwaitingPrompt {
		t.Fatalf("review did not retain its pending prompt anchor: %#v", state)
	}
	application.renderTerminalWorkspace()

	application.processRegularSSHOutput([]byte("\x1b[32mroot@server:~# \x1b[0m"))
	if state.AwaitingPrompt || state.AnchorX != 15 || state.AnchorY != 1 {
		t.Fatalf("review does not align with the newly received prompt: %#v", state)
	}
	if application.terminalRows != nil {
		t.Fatal("old review position was not invalidated")
	}
	layout, ok := application.inlineLayoutFor(state)
	if !ok || layout.Left != 16 {
		t.Fatalf("review starts at column %d, want 16", layout.Left)
	}

	// A later prompt must not pull the existing review away from its command.
	application.processRegularSSHOutput([]byte("\r\nnext$ "))
	if state.AnchorX != 15 || state.AnchorY != 1 {
		t.Fatalf("later prompt moved the completed review: %#v", state)
	}
}
