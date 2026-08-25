package app

import (
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
