package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestAIEditorArrowKeysKeepFocusAndMoveCursor(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.focus = focusAI
	application.handleAIInput([]byte("abc"))
	application.handleAIInput([]byte("\x1b[D"))
	if application.focus != focusAI || application.aiEditorCursor != 2 {
		t.Fatalf("left arrow changed focus or cursor: focus=%v cursor=%d", application.focus, application.aiEditorCursor)
	}
	application.handleAIInput([]byte("X"))
	if got := string(application.editor); got != "abXc" {
		t.Fatalf("insert after moving left = %q, want %q", got, "abXc")
	}
	application.handleAIInput([]byte("\x1b[C"))
	if application.aiEditorCursor != 4 {
		t.Fatalf("right arrow cursor=%d, want 4", application.aiEditorCursor)
	}
}

func TestAIEditorSupportsMultilineInput(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.focus = focusAI
	application.handleAIInput([]byte("第一行"))
	application.handleAIInput([]byte{'\n'}) // Ctrl+J
	application.handleAIInput([]byte("第二行"))
	application.handleAIInput([]byte("\x1b[13;2u")) // Shift+Enter in CSI-u terminals.
	application.handleAIInput([]byte("第三行"))
	if got := string(application.editor); got != "第一行\n第二行\n第三行" {
		t.Fatalf("multiline editor=%q", got)
	}

	layout := application.buildAIEditorLayout(50, 3)
	if got := strings.Join(layout.Lines, "\n"); !strings.Contains(got, "› 第一行") || !strings.Contains(got, "↳ 第三行") {
		t.Fatalf("multiline layout=%q", got)
	}
}

func TestAIInputViewportAndScrollStayStableWhenFocused(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	if before, after := aiConversationViewport(12, false), aiConversationViewport(12, true); before != after {
		t.Fatalf("conversation viewport jumped on focus: before=%d after=%d", before, after)
	}
	application.aiScroll = 7
	application.openAIConversation()
	if application.aiScroll != 7 {
		t.Fatalf("opening AI reset scroll=%d", application.aiScroll)
	}
}

func TestAIInputIsVisibleWithoutFocusAndUsesBottomRow(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.focus = focusTerminal
	application.aiExpanded = true
	application.editor = []rune("第一行\n第二行\n第三行")
	application.aiEditorCursor = len(application.editor)
	const top, left, width, height = 3, 90, 50, 12
	output := application.renderAIWorkspace(top, left, width, height)
	for _, want := range []string{"╭─ 输入", "› 第一行", "↳ 第二行", "↳ 第三行"} {
		if !strings.Contains(output, want) {
			t.Fatalf("unfocused AI input missing %q: %q", want, output)
		}
	}
	lastRow := fmt.Sprintf("\x1b[%d;%dH", top+height-1, left)
	index := strings.LastIndex(output, lastRow)
	if index < 0 || !strings.Contains(output[index:], "↳ 第三行") {
		t.Fatalf("last input line was not rendered on bottom row: %q", output)
	}
}

func TestAIBracketedPasteKeepsEmbeddedNewlines(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.focus = focusAI
	application.handleAIInput([]byte("\x1b[200~first\r\nsecond\x1b[201~"))
	if got := string(application.editor); got != "first\nsecond" {
		t.Fatalf("pasted editor=%q", got)
	}
	if application.aiPasteMode {
		t.Fatal("bracketed paste mode was not closed")
	}
}
