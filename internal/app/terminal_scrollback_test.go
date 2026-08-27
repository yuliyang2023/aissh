package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

func TestTerminalOutputCapturesRowsThatLeaveViewport(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 20
	application.contentRows = 3
	application.rows = 5
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))

	history := application.writeTerminalOutput([]byte("one\r\ntwo\r\nthree\r\nfour\r\n"))
	if len(history) != 2 {
		t.Fatalf("history lines=%d, expected 2", len(history))
	}
	first := terminalview.RenderLine(history[0], 1, 1, application.mainCols)
	second := terminalview.RenderLine(history[1], 1, 1, application.mainCols)
	if !strings.Contains(first, "one") || !strings.Contains(second, "two") {
		t.Fatalf("captured history=%q / %q", first, second)
	}
}

func TestTerminalScrollKeepsPositionWhenOutputContinues(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.terminalHistory = make([]terminalview.Line, 10)
	application.terminalScroll = 5
	application.appendTerminalHistory([]terminalview.Line{{}, {}})
	if application.terminalScroll != 7 {
		t.Fatalf("scroll=%d, expected anchored offset 7", application.terminalScroll)
	}
	application.scrollTerminal(false)
	if application.terminalScroll != 4 {
		t.Fatalf("scroll down=%d", application.terminalScroll)
	}
}

func TestTerminalHistoryUsesConfiguredLimit(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, ScrollbackLines: 4})
	if err != nil {
		t.Fatal(err)
	}
	application.appendTerminalHistory(make([]terminalview.Line, 7))
	if len(application.terminalHistory) != 4 {
		t.Fatalf("history lines=%d", len(application.terminalHistory))
	}
}

func TestScrollbarStaysInGutterOffPaneContent(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 20
	application.contentRows = 3
	application.rows = 5
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	application.terminalHistory = make([]terminalview.Line, 10)

	// The scrollbar owns the gutter column right of the pane content, so a rune
	// in the pane's last column (e.g. the second half of a CJK rune) survives.
	output, _, _ := application.renderTerminalWorkspace()
	gutterThumb := fmt.Sprintf(";%dH\x1b[38;5;45m█", application.terminalScrollbarCol())
	if !strings.Contains(output, gutterThumb) {
		t.Fatalf("scrollbar thumb missing from gutter column %d: %q", application.terminalScrollbarCol(), output)
	}
	paneThumb := fmt.Sprintf(";%dH\x1b[38;5;45m█", application.mainCols)
	if strings.Contains(output, paneThumb) {
		t.Fatalf("scrollbar thumb painted over pane content column %d: %q", application.mainCols, output)
	}

	// Without scrollback the gutter is blanked rather than left stale.
	application.terminalHistory = nil
	application.terminalRows = nil
	output, _, _ = application.renderTerminalWorkspace()
	gutterBlank := fmt.Sprintf(";%dH\x1b[0m ", application.terminalScrollbarCol())
	if strings.Contains(output, "█") || !strings.Contains(output, gutterBlank) {
		t.Fatalf("gutter not blanked without scrollback: %q", output)
	}
}

func TestScrollbarGutterClickJumpsScroll(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 120
	application.rows = 30
	application.mainCols = 76
	application.contentRows = 28
	application.terminalHistory = make([]terminalview.Line, 40)

	application.handleLeftClick(application.terminalScrollbarCol(), 2)
	if application.terminalScroll == 0 || application.focus != focusTerminal {
		t.Fatalf("gutter click did not jump scroll: scroll=%d focus=%v", application.terminalScroll, application.focus)
	}
}

func TestScrollbarThumbMovesFromBottomToTop(t *testing.T) {
	bottomTop, size := scrollbarThumb(20, 100, 20, 0)
	if bottomTop+size != 20 {
		t.Fatalf("bottom thumb top=%d size=%d", bottomTop, size)
	}
	top, topSize := scrollbarThumb(20, 100, 20, 80)
	if top != 0 || topSize != size {
		t.Fatalf("top thumb top=%d size=%d", top, topSize)
	}
}
