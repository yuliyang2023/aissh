package app

import (
	"bytes"
	"encoding/base64"
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

func TestPlainDragSelectsAndRightClickCopiesWithoutShift(t *testing.T) {
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

	application.handleMouse(mouseEvent{Button: 2, X: 9, Y: 4})
	if application.textSelection.Active || application.nativeSelection {
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
}

func TestSplitMouseInputEventsKeepDragSelectionState(t *testing.T) {
	application := mouseTestApp(t)
	application.selectionCanvas = newScreenTextCanvas(application.rows, application.cols)
	application.selectionCanvas.apply("\x1b[4;5Hhello world", application.rows, application.cols)

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
