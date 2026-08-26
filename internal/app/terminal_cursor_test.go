package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hinshun/vt10x"
)

func TestSSHCursorRemainsVisibleWhileInlineAIIsBusy(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 120
	application.rows = 30
	application.mainCols = 76
	application.contentRows = 28
	application.focus = focusTerminal
	application.atPrompt = true
	application.agentBusy = true
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	_, _ = application.virtual.Write([]byte("root@server:~# "))
	application.inlineAI = &inlineAIState{
		Prompt: "检查服务", AnchorX: 15, AnchorY: 0, Status: "正在处理", Started: time.Now(),
	}
	var screen bytes.Buffer
	application.screenOut = &screen

	application.render()
	output := screen.String()
	showCursor := strings.LastIndex(output, "\x1b[?25h")
	hideCursor := strings.LastIndex(output, "\x1b[?25l")
	if showCursor < 0 || showCursor < hideCursor {
		t.Fatalf("SSH cursor was not restored while AI was busy: %q", output)
	}
}
