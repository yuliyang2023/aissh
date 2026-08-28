package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestScreenTextCanvasTracksPositionedUnicodeText(t *testing.T) {
	canvas := newScreenTextCanvas(5, 30)
	canvas.apply("\x1b[2;3H\x1b[38;5;45mabc中文\x1b[0m\x1b[3;1Hnext", 5, 30)
	if got := strings.TrimSpace(canvas.cellText(2, 3, 12)); got != "abc中文" {
		t.Fatalf("row 2=%q", got)
	}
	if got := strings.TrimSpace(canvas.cellText(3, 1, 8)); got != "next" {
		t.Fatalf("row 3=%q", got)
	}
}

func TestPlainDragSelectsAndCopiesWithoutShift(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	var screen bytes.Buffer
	application.screenOut = &screen

	application.handleMouse(mouseEvent{Button: 0, X: 5, Y: 4})
	application.handleMouse(mouseEvent{Button: 32, X: 9, Y: 4, Motion: true})
	application.handleMouse(mouseEvent{Button: 0, X: 9, Y: 4, Release: true})
	if !application.textSelection.Active || application.selectedScreenText() != "hello" {
		t.Fatalf("selection=%#v text=%q", application.textSelection, application.selectedScreenText())
	}
	if application.nativeSelection {
		t.Fatalf("selection=%#v native=%v", application.textSelection, application.nativeSelection)
	}
	output := screen.String()
	prefix := "\x1b]52;c;"
	start := strings.Index(output, prefix)
	end := strings.Index(output, "\x07")
	if start < 0 || end < start {
		t.Fatalf("OSC 52 copy missing: %q", output)
	}
	decoded, err := base64.StdEncoding.DecodeString(output[start+len(prefix) : end])
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("clipboard=%q err=%v", decoded, err)
	}
	if status := application.inputStatus(); !strings.Contains(status, "已选择并复制 5 个字符") {
		t.Fatalf("status=%q", status)
	}
}

func TestITermDragUsesNativePasteboardEscape(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("LC_TERMINAL", "iTerm2")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	var screen bytes.Buffer
	application.screenOut = &screen

	application.handleMouse(mouseEvent{Button: 0, X: 5, Y: 4})
	application.handleMouse(mouseEvent{Button: 32, X: 9, Y: 4, Motion: true})
	application.handleMouse(mouseEvent{Button: 0, X: 9, Y: 4, Release: true})

	output := screen.String()
	prefix := "\x1b]1337;Copy=:"
	start := strings.Index(output, prefix)
	end := strings.Index(output, "\x07")
	if start < 0 || end < start {
		t.Fatalf("iTerm2 copy missing: %q", output)
	}
	decoded, err := base64.StdEncoding.DecodeString(output[start+len(prefix) : end])
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("clipboard=%q err=%v", decoded, err)
	}
	if !application.textSelection.Active {
		t.Fatal("copy unexpectedly cleared the visual selection")
	}
}

func TestAppleTerminalDragUsesMacPasteboard(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	var copied string
	application.clipboardWrite = func(value string) error {
		copied = value
		return nil
	}
	var screen bytes.Buffer
	application.screenOut = &screen

	application.handleMouse(mouseEvent{Button: 0, X: 5, Y: 4})
	application.handleMouse(mouseEvent{Button: 32, X: 9, Y: 4, Motion: true})
	application.handleMouse(mouseEvent{Button: 0, X: 9, Y: 4, Release: true})

	if copied != "hello" {
		t.Fatalf("pasteboard=%q", copied)
	}
	if screen.Len() != 0 {
		t.Fatalf("Apple Terminal unexpectedly received a clipboard escape: %q", screen.String())
	}
}

func TestAppleTerminalFallsBackToOSC52WhenPasteboardFails(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	application.textSelection = screenTextSelection{
		Start:  screenPoint{X: 5, Y: 4},
		End:    screenPoint{X: 9, Y: 4},
		Active: true,
	}
	application.clipboardWrite = func(string) error {
		return errors.New("pasteboard unavailable")
	}
	var screen bytes.Buffer
	application.screenOut = &screen

	application.copyTextSelection()

	if !strings.Contains(screen.String(), "\x1b]52;c;") {
		t.Fatalf("OSC 52 fallback missing: %q", screen.String())
	}
}

func TestWindowsTerminalDragUsesOSC52(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "windows-terminal-session")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	var screen bytes.Buffer
	application.screenOut = &screen

	application.handleMouse(mouseEvent{Button: 0, X: 5, Y: 4})
	application.handleMouse(mouseEvent{Button: 32, X: 9, Y: 4, Motion: true})
	application.handleMouse(mouseEvent{Button: 0, X: 9, Y: 4, Release: true})

	output := screen.String()
	prefix := "\x1b]52;c;"
	start := strings.Index(output, prefix)
	end := strings.Index(output, "\x1b\\")
	if start < 0 || end < start {
		t.Fatalf("Windows Terminal OSC 52 missing: %q", output)
	}
	decoded, err := base64.StdEncoding.DecodeString(output[start+len(prefix) : end])
	if err != nil || string(decoded) != "hello" {
		t.Fatalf("clipboard=%q err=%v", decoded, err)
	}
}

func TestControlCCopiesActiveVisualSelectionInsteadOfInterrupting(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("LC_TERMINAL", "iTerm2")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	application.textSelection = screenTextSelection{
		Start:  screenPoint{X: 5, Y: 4},
		End:    screenPoint{X: 9, Y: 4},
		Active: true,
	}
	var screen bytes.Buffer
	application.screenOut = &screen

	application.handleInput([]byte{0x03})

	if !application.textSelection.Active {
		t.Fatal("Ctrl+C cleared the visual selection instead of copying it")
	}
	if !strings.Contains(screen.String(), "\x1b]1337;Copy=:") {
		t.Fatalf("copy escape missing: %q", screen.String())
	}
}

func TestSplitMouseInputEventsKeepDragSelectionState(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("LC_TERMINAL", "")
	t.Setenv("TMUX", "")
	t.Setenv("WT_SESSION", "")
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)
	application.screenOut = &bytes.Buffer{}

	application.handleInput([]byte("\x1b[<0;5;4M"))
	if !application.textSelection.Dragging {
		t.Fatal("mouse press did not start selection")
	}
	application.handleInput([]byte("\x1b[<32;9;4M"))
	if !application.textSelection.Dragging || !application.textSelection.Active {
		t.Fatalf("mouse motion lost selection state: %#v", application.textSelection)
	}
	application.handleInput([]byte("\x1b[<0;9;4m"))
	if application.textSelection.Dragging || !application.textSelection.Active || application.selectedScreenText() != "hello" {
		t.Fatalf("completed selection=%#v text=%q", application.textSelection, application.selectedScreenText())
	}
}

func TestClickWithoutDragStillActivatesPanelAndDoesNotSelect(t *testing.T) {
	application := mouseTestApp(t)
	top, left, _, _ := application.aiRegion()
	application.handleMouse(mouseEvent{Button: 0, X: left + 2, Y: top + 1})
	application.handleMouse(mouseEvent{Button: 0, X: left + 2, Y: top + 1, Release: true})
	if application.focus != focusAI || application.textSelection.Active {
		t.Fatalf("focus=%v selection=%#v", application.focus, application.textSelection)
	}
}

func TestParseSGRButtonMotion(t *testing.T) {
	event, ok := parseSGRMouse([]byte("\x1b[<32;20;7M"))
	if !ok || !event.Motion || event.Release || event.Button&3 != 0 {
		t.Fatalf("motion=%#v ok=%v", event, ok)
	}
}
