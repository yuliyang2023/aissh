package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hinshun/vt10x"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

func TestSidebarStartsCollapsedAndRestoresLayout(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()
	if !application.sidebarCollapsed || application.mainCols != 139 || application.sidebarVisible() {
		t.Fatalf("collapsed=%v mainCols=%d visible=%v", application.sidebarCollapsed, application.mainCols, application.sidebarVisible())
	}
	if button := application.sidebarToggleButton(); !strings.Contains(button, "展开右栏") {
		t.Fatalf("collapsed button=%q", button)
	}

	application.toggleSidebar()
	if application.sidebarCollapsed || application.mainCols >= 139 || !application.sidebarVisible() {
		t.Fatalf("collapsed=%v mainCols=%d visible=%v", application.sidebarCollapsed, application.mainCols, application.sidebarVisible())
	}
	expandedMainCols := application.mainCols
	application.toggleSidebar()
	if !application.sidebarCollapsed || application.mainCols != 139 {
		t.Fatalf("collapsed=%v mainCols=%d", application.sidebarCollapsed, application.mainCols)
	}
	application.toggleSidebar()
	if application.mainCols != expandedMainCols {
		t.Fatalf("restored width=%d want=%d", application.mainCols, expandedMainCols)
	}
}

func TestCtrlShiftMinusTogglesSidebarOnlyAtShellPrompt(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	application.sshPTY = writer

	shortcut := []byte{0x1f}
	application.handleKeyboardInput(shortcut)
	if !application.sidebarCollapsed {
		t.Fatal("Ctrl+Shift+- was intercepted away from a shell prompt")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if string(written) != string(shortcut) {
		t.Fatalf("non-prompt Ctrl+Shift+- was not forwarded to SSH: %q", written)
	}
	application.sshPTY = nil

	application.atPrompt = true
	application.handleKeyboardInput(shortcut)
	if application.sidebarCollapsed || application.focus != focusTerminal {
		t.Fatalf("collapsed=%v focus=%v", application.sidebarCollapsed, application.focus)
	}

	application.handleKeyboardInput(shortcut)
	if !application.sidebarCollapsed {
		t.Fatal("Ctrl+Shift+- did not hide the sidebar")
	}
}

func TestSidebarHeaderButtonCanBeClicked(t *testing.T) {
	application := mouseTestApp(t)
	start := application.sidebarToggleHeaderStart()
	if start <= 0 {
		t.Fatal("sidebar toggle button missing")
	}
	clickMouse(application, start, 1)
	if !application.sidebarCollapsed || application.mainCols != application.cols-1 {
		t.Fatalf("collapsed=%v mainCols=%d", application.sidebarCollapsed, application.mainCols)
	}
	clickMouse(application, application.sidebarToggleHeaderStart(), 1)
	if application.sidebarCollapsed {
		t.Fatal("header button did not expand the sidebar")
	}
}

func TestOpeningSidebarWorkspaceExpandsCollapsedSidebar(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()

	application.openAIConversation()
	if application.sidebarCollapsed || application.focus != focusAI || !application.sidebarVisible() {
		t.Fatalf("collapsed=%v focus=%v visible=%v", application.sidebarCollapsed, application.focus, application.sidebarVisible())
	}
}

func TestWideResizeShowsFocusedCompactWorkspace(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.setViewportSize(140, 30)
	application.setViewportSize(80, 30)
	application.focus = focusAI
	application.setViewportSize(140, 30)

	if application.sidebarCollapsed || !application.sidebarVisible() || application.focus != focusAI {
		t.Fatalf("collapsed=%v visible=%v focus=%v", application.sidebarCollapsed, application.sidebarVisible(), application.focus)
	}
}

func sidebarReplyTestApp(t *testing.T) *App {
	t.Helper()
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.cancel)
	application.setViewportSize(140, 30)
	application.ensureSidebarVisible()
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	application.processSSHOutput([]byte("root@server:~# "))
	return application
}

func assertCollapsedReplyVisible(t *testing.T, application *App, want string) {
	t.Helper()
	if !application.sidebarCollapsed || application.sidebarVisible() {
		t.Fatal("reply reopened the sidebar")
	}
	application.renderTerminalWorkspace()
	if frame := cachedTerminalRows(application.terminalRows, application.screenRows()); !strings.Contains(frame, want) {
		t.Fatalf("terminal reply missing %q: %q", want, frame)
	}
}

func TestSidebarResizeRedrawPreservesInlineReply(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			application := sidebarReplyTestApp(t)
			cursor := application.virtual.Cursor()
			state := &inlineAIState{Prompt: "检查服务", Response: "服务正常", AnchorX: cursor.X, AnchorY: cursor.Y, Done: !streaming, Started: time.Now()}
			application.inlineAI = state
			if streaming {
				application.agentBusy = true
				application.agentInline = state
			}
			application.handleKeyboardInput([]byte{0x1f})
			// A ConPTY resize can repaint the screen, with the clear sequence and
			// the final shell prompt arriving in separate reads.
			for _, chunk := range []string{"\x1b[2", "J\x1b[H", "\r\nroot@server:~# "} {
				application.processSSHOutput([]byte(chunk))
			}
			if streaming {
				application.handleEvent(agentAnswerEvent{content: "，日志正常"})
				application.handleEvent(agentTurnEvent{turn: agent.Turn{Type: "finished", Message: "服务正常，日志正常"}})
			}
			if application.inlineAI != state {
				t.Fatal("resize detached the inline reply from the agent")
			}
			assertCollapsedReplyVisible(t, application, "服务正常")
		})
	}
}

func TestSidebarResizeReanchorsConversationAfterCursorRestore(t *testing.T) {
	for _, clear := range []bool{false, true} {
		t.Run(fmt.Sprintf("clear=%v", clear), func(t *testing.T) {
			application := sidebarReplyTestApp(t)
			prompt := "root@server:~# "
			application.processSSHOutput([]byte("\r" + strings.Repeat("output\r\n", application.contentRows-1) + prompt))
			cursor := application.virtual.Cursor()
			older := &inlineAIState{Prompt: "第一次检查", Response: "第一段开始\n第一段结尾", AnchorX: cursor.X, AnchorY: cursor.Y, Done: true}
			latest := &inlineAIState{Prompt: "再次确认", Response: "第二段开始\n第二段结尾", AnchorX: cursor.X, AnchorY: cursor.Y, Done: true}
			application.inlineHistory = []*inlineAIState{older}
			application.inlineAI = latest

			for _, promptRow := range []int{6, 18, 4} {
				application.toggleSidebar()
				prefix := "\x1b[H"
				if clear {
					prefix = "\x1b[2J" + prefix
				}
				// ConPTY can repaint text, visit blank rows, then restore the
				// input cursor in a later read containing no prompt text.
				application.processSSHOutput([]byte(prefix + fmt.Sprintf("\x1b[%d;1H%s\x1b[%d;1H", promptRow, prompt, application.contentRows)))
				application.processSSHOutput([]byte(fmt.Sprintf("\x1b[%d;%dH", promptRow, len(prompt)+1)))
				for _, state := range []*inlineAIState{older, latest} {
					if state.AnchorY != promptRow-1 || state.AnchorX != len(prompt) {
						t.Fatalf("reply %q stayed at (%d,%d), want (%d,%d)", state.Prompt, state.AnchorX, state.AnchorY, len(prompt), promptRow-1)
					}
				}
				application.renderTerminalWorkspace()
				frame := cachedTerminalRows(application.terminalRows, application.screenRows())
				for _, want := range []string{older.Prompt, "第一段结尾", latest.Prompt, "第二段结尾"} {
					if !strings.Contains(frame, want) {
						t.Fatalf("repaint clipped %q", want)
					}
				}
			}
		})
	}
}

func TestSidebarResizeRepaintPreservesHistoryAcrossScrolling(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.processSSHOutput([]byte("\r" + strings.Repeat("output\r\n", application.contentRows-1) + "root@server:~# "))
	cursor := application.virtual.Cursor()
	older := &inlineAIState{Prompt: "第一次检查", Response: "历史结论", AnchorX: cursor.X, AnchorY: cursor.Y - 2, Done: true}
	latest := &inlineAIState{Prompt: "再次确认", Response: "最新结论", AnchorX: cursor.X, AnchorY: cursor.Y, Done: true}
	application.inlineHistory = []*inlineAIState{older}
	application.inlineAI = latest
	application.toggleSidebar()
	application.processSSHOutput([]byte("\x1b[2J\x1b[H" + strings.Repeat("repaint\r\n", 2*application.contentRows)))
	application.processSSHOutput([]byte("root@server:~# "))
	if len(application.inlineHistory) != 1 || application.inlineHistory[0] != older {
		t.Fatal("resize repaint discarded the previous reply")
	}
	if older.AnchorY != latest.AnchorY-2 {
		t.Fatalf("reply spacing changed: older=%d latest=%d", older.AnchorY, latest.AnchorY)
	}
	assertCollapsedReplyVisible(t, application, "历史结论")
	assertCollapsedReplyVisible(t, application, "最新结论")
	// Once a real command starts, normal terminal scrolling still retires
	// replies that have left the viewport.
	application.beginCommandOutput("seq 100")
	application.processSSHOutput([]byte("\r\n" + strings.Repeat("output\r\n", application.contentRows+2)))
	if len(application.inlineHistory) != 0 || application.inlineAI != nil {
		t.Fatal("normal command scrolling retained offscreen replies")
	}
}

func TestSidebarResizeDuringCommandKeepsNormalScrolling(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.inlineHistory = []*inlineAIState{{Prompt: "历史问题", AnchorY: 0, Done: true}}
	application.inlineAI = &inlineAIState{Prompt: "最近问题", AnchorY: 0, Done: true}
	application.beginCommandOutput("tail -f server.log")
	application.atPrompt = false
	application.toggleSidebar()
	application.processSSHOutput([]byte("\r\n" + strings.Repeat("log output\r\n", 2*application.contentRows)))
	if len(application.inlineHistory) != 0 || application.inlineAI != nil {
		t.Fatal("resizing during a command froze replies over ongoing output")
	}
}

func TestSidebarResizeDuringRepaintKeepsOriginalAnchor(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.processSSHOutput([]byte("\r" + strings.Repeat("output\r\n", 12) + "root@server:~# "))
	cursor := application.virtual.Cursor()
	application.inlineAI = &inlineAIState{Prompt: "检查服务", Response: "完整结论", AnchorX: cursor.X, AnchorY: cursor.Y, Done: true}
	application.toggleSidebar()
	application.processSSHOutput([]byte("\x1b[2J\x1b[H"))
	// Another layout change may happen while the first repaint's cursor is
	// temporarily at the top, before either repaint restores the shell input.
	application.toggleSidebar()
	application.toggleSidebar()
	application.processSSHOutput([]byte("\x1b[5;1Hroot@server:~# "))
	if application.inlineAI.AnchorY != 4 {
		t.Fatalf("reply followed a transient repaint cursor: row=%d", application.inlineAI.AnchorY)
	}
	assertCollapsedReplyVisible(t, application, "完整结论")
}

func TestCollapseSidebarMirrorsStreamingReply(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.chat = append(application.chat, chatMessage{Role: "user", Content: "检查服务"})
	application.agentBusy = true
	application.focus = focusAI
	application.handleEvent(agentAnswerEvent{content: "服务正常"})
	application.handleKeyboardInput([]byte{0x1f})
	assertCollapsedReplyVisible(t, application, "服务正常")
	state := application.inlineAI
	application.handleEvent(agentAnswerEvent{content: "，日志正常"})
	application.handleEvent(agentTurnEvent{turn: agent.Turn{Type: "finished", Message: "服务正常，日志正常"}})
	assertCollapsedReplyVisible(t, application, "服务正常，日志正常")
	application.toggleSidebar()
	application.toggleSidebar()
	if application.inlineAI != state || len(application.inlineHistory) != 0 {
		t.Fatal("repeated sidebar toggles duplicated the inline reply")
	}
}

func TestCollapsedSidebarShowsForegroundAIReply(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.toggleSidebar()
	application.agent = turnTestProvider{turn: agent.Turn{Type: "finished", Message: "未找到命令，请检查拼写"}}
	application.pendingError = &trigger.ErrorMatch{Kind: "command_not_found", Message: "badcmd: command not found"}
	application.handleEvent(delayedErrorTriggerEvent{match: *application.pendingError, command: "badcmd"})
	select {
	case event := <-application.events:
		application.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("AI did not finish")
	}
	assertCollapsedReplyVisible(t, application, "未找到命令，请检查拼写")
}

func TestSidebarResizeRedrawAfterLocallyCapturedAIInput(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.toggleSidebar()
	application.agent = turnTestProvider{turn: agent.Turn{Type: "finished", Message: "服务正常"}}
	application.handleKeyboardInput([]byte("检查服务\r"))
	select {
	case event := <-application.events:
		application.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("AI did not finish")
	}
	// Resizing and ConPTY output are asynchronous; even a completed local
	// question must survive a late repaint of the remote shell.
	application.processSSHOutput([]byte("\x1b[2J\x1b[H" + strings.Repeat("line\r\n", application.contentRows+2) + "root@server:~# "))
	assertCollapsedReplyVisible(t, application, "服务正常")
	if application.inlineAI.AnchorY != application.virtual.Cursor().Y {
		t.Fatal("reply did not follow the repainted prompt")
	}
}

func TestSidebarResizeStillAllowsTerminalClear(t *testing.T) {
	for _, action := range []string{"ctrl-l", "clear", "remote-command"} {
		t.Run(action, func(t *testing.T) {
			application := sidebarReplyTestApp(t)
			application.inlineAI = &inlineAIState{Prompt: "检查服务", Response: "服务正常", Done: true}
			application.chat = append(application.chat, chatMessage{Role: "assistant", Content: "服务正常"})
			application.toggleSidebar()
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			application.sshPTY = writer
			switch action {
			case "ctrl-l":
				application.handleKeyboardInput([]byte{0x0c})
			case "clear":
				application.handleKeyboardInput([]byte("clear\r"))
			case "remote-command":
				application.handleKeyboardInput([]byte("./redraw-screen\r"))
			}
			application.processSSHOutput([]byte("\x1b[2J\x1b[Hroot@server:~# "))
			if application.inlineAI != nil || len(application.inlineHistory) != 0 {
				t.Fatal("terminal clear kept stale inline replies after resize")
			}
			if application.chat[len(application.chat)-1].Content != "服务正常" {
				t.Fatal("terminal clear erased the synchronized conversation")
			}
		})
	}
}

func TestCollapseSidebarKeepsPendingApprovalFocused(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.proposal = &agent.Proposal{ID: "proposal", Command: "uptime", Risk: "read_only"}
	application.focus = focusAI
	application.handleKeyboardInput([]byte{0x1f})
	if application.focus != focusApproval {
		t.Fatal("collapsing the sidebar left approval input in a hidden pane")
	}
}

func TestCollapsedSidebarShowsNonInlineExecutionResult(t *testing.T) {
	application := sidebarReplyTestApp(t)
	application.toggleSidebar()
	application.agent = turnTestProvider{turn: agent.Turn{Type: "finished", Message: "命令执行成功"}}
	exitCode := 0
	application.execution = &executionState{Command: "uptime", ExitCode: &exitCode}
	application.submitExecutionResult()
	select {
	case event := <-application.events:
		application.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("execution analysis did not finish")
	}
	assertCollapsedReplyVisible(t, application, "命令执行成功")
}
