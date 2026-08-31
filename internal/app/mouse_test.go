package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	chat_history "git.example.com/ai/wrd-ai-cli/internal/history"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

func TestParseSGRMouse(t *testing.T) {
	event, ok := parseSGRMouse([]byte("\x1b[<0;103;7M"))
	if !ok || event.Button != 0 || event.X != 103 || event.Y != 7 || event.Release {
		t.Fatalf("event=%#v ok=%v", event, ok)
	}
	release, ok := parseSGRMouse([]byte("\x1b[<0;103;7m"))
	if !ok || !release.Release {
		t.Fatalf("release=%#v ok=%v", release, ok)
	}
}

func TestMouseClicksMonitorTabAddAndAI(t *testing.T) {
	application := mouseTestApp(t)
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	_, left, width, _ := application.monitorRegion()

	processX := 0
	addX := 0
	for x := left; x < left+width; x++ {
		id, add, ok := application.monitorHeaderTarget(x)
		if ok && id == "process" {
			processX = x
		}
		if ok && add {
			addX = x
		}
	}
	if processX == 0 || addX == 0 {
		t.Fatalf("process=%d add=%d", processX, addX)
	}
	monitorTop, _, _, _ := application.monitorRegion()
	clickMouse(application, processX, monitorTop)
	if application.panels.Active().ID != "process" || application.focus != focusMonitor {
		t.Fatalf("active=%s focus=%v", application.panels.Active().ID, application.focus)
	}
	clickMouse(application, addX, monitorTop)
	if application.focus != focusPanelMenu {
		t.Fatalf("add focus=%v", application.focus)
	}

	aiTop, aiLeft, _, _ := application.aiRegion()
	clickMouse(application, aiLeft+1, aiTop)
	if application.focus != focusAI || !application.aiExpanded {
		t.Fatalf("ai focus=%v expanded=%v", application.focus, application.aiExpanded)
	}
}

func TestMouseWheelScrollsAIHistory(t *testing.T) {
	application := mouseTestApp(t)
	for index := 0; index < 20; index++ {
		application.chat = append(application.chat, chatMessage{Role: "assistant", Content: strings.Repeat("回答", 20), Time: time.Now()})
	}
	application.aiExpanded = true
	aiTop, aiLeft, _, _ := application.aiRegion()
	application.handleMouse(mouseEvent{Button: 64, X: aiLeft + 1, Y: aiTop + 1})
	if application.aiScroll == 0 || application.focus != focusAI {
		t.Fatalf("scroll=%d focus=%v", application.aiScroll, application.focus)
	}
	application.handleMouse(mouseEvent{Button: 65, X: aiLeft + 1, Y: aiTop + 1})
	if application.aiScroll != 0 {
		t.Fatalf("scroll down=%d", application.aiScroll)
	}
}

func TestMouseClickOnInlineAIOpensConversation(t *testing.T) {
	application := mouseTestApp(t)
	application.inlineAI = &inlineAIState{
		Prompt: "检查 nginx", AnchorX: 12, AnchorY: 2,
		Response: "nginx 正常", Done: true, Started: time.Now(),
	}
	layout, ok := application.inlineLayoutFor(application.inlineAI)
	if !ok {
		t.Fatal("inline AI layout missing")
	}
	clickMouse(application, layout.Left+1, layout.Top+1)
	if application.focus != focusAI || !application.aiExpanded {
		t.Fatalf("inline click focus=%v expanded=%v", application.focus, application.aiExpanded)
	}
}

func TestMouseClicksInlineCommandFoldToExpand(t *testing.T) {
	application := mouseTestApp(t)
	proposal := &agent.Proposal{
		ID:      "inline-fold",
		Command: "journalctl -u nginx --since today --no-pager | grep error | tail -n 200 && systemctl status nginx --no-pager && ss -lntp | grep nginx && ps aux --sort=-%cpu | head",
		Risk:    "read_only",
	}
	application.inlineAI = &inlineAIState{Prompt: "检查 nginx", AnchorX: 18, AnchorY: 2, Proposal: proposal}
	application.proposal = proposal
	application.focus = focusApproval
	layout, ok := application.inlineLayoutFor(application.inlineAI)
	if !ok {
		t.Fatal("inline proposal layout missing")
	}
	clicked := false
	for index, line := range layout.Lines {
		marker := strings.Index(line, "[展开全部]")
		if marker < 0 {
			continue
		}
		clickMouse(application, layout.Left+textWidth(line[:marker])+1, layout.Top+1+index)
		clicked = true
		break
	}
	if !clicked || !application.inlineAI.CommandExpanded || application.focus != focusApproval {
		t.Fatalf("clicked=%v expanded=%v focus=%v", clicked, application.inlineAI.CommandExpanded, application.focus)
	}
}

func TestMouseClicksAIConversationCommandFoldToExpand(t *testing.T) {
	application := mouseTestApp(t)
	application.chat = []chatMessage{{
		Role: "proposal",
		Proposal: &agent.Proposal{
			ID:      "chat-fold",
			Command: "journalctl -u nginx --since today --no-pager | grep error | tail -n 200 && systemctl status nginx --no-pager && ss -lntp | grep nginx && ps aux --sort=-%cpu | head",
			Risk:    "read_only",
		},
	}}
	application.focus = focusTerminal
	top, left, width, height := application.aiRegion()
	visible := fitTailDisplayOffset(application.aiDisplayLines(width-2), aiConversationViewport(height, false), 0)
	clicked := false
	for row, line := range visible {
		if !line.ExpandCommand {
			continue
		}
		marker := strings.Index(line.Text, "[展开全部]")
		if marker < 0 {
			continue
		}
		clickMouse(application, left+1+textWidth(line.Text[:marker])+1, top+1+row)
		clicked = true
		break
	}
	if !clicked || !application.chat[0].CommandExpanded || application.focus != focusTerminal {
		t.Fatalf("clicked=%v expanded=%v focus=%v", clicked, application.chat[0].CommandExpanded, application.focus)
	}
}

func TestMouseOpensAndSelectsAIHistoryConversation(t *testing.T) {
	directory := t.TempDir()
	store := chat_history.New(directory)
	if err := store.Append("server", chat_history.Message{
		ConversationID: "nginx", Role: "user", Content: "检查 nginx", Time: "2026-08-25T10:00:00+08:00",
	}); err != nil {
		t.Fatal(err)
	}
	application, err := New(Config{SSHArgs: []string{"server"}, HistoryDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.mainCols = 89
	application.contentRows = 34
	aiTop, _, _, _ := application.aiRegion()
	clickMouse(application, application.aiHistoryHeaderStart(), aiTop)
	if application.focus != focusAIHistory {
		t.Fatalf("focus=%v", application.focus)
	}
	// The first row creates a new session; the second row is the newest saved session.
	clickMouse(application, application.aiHistoryHeaderStart()-2, aiTop+2)
	if application.focus != focusAI || application.historyConversationID != "nginx" {
		t.Fatalf("focus=%v conversation=%q", application.focus, application.historyConversationID)
	}
}

func TestMouseWheelScrollsSSHHistory(t *testing.T) {
	application := mouseTestApp(t)
	application.terminalHistory = make([]terminalview.Line, 20)
	application.focus = focusTerminal
	application.handleMouse(mouseEvent{Button: 64, X: 10, Y: 5})
	if application.terminalScroll != 3 || application.focus != focusTerminal {
		t.Fatalf("scroll=%d focus=%v", application.terminalScroll, application.focus)
	}
	application.handleMouse(mouseEvent{Button: 65, X: 10, Y: 5})
	if application.terminalScroll != 0 {
		t.Fatalf("scroll down=%d", application.terminalScroll)
	}
}

func TestHandleInputRoutesReportedMouseWheel(t *testing.T) {
	application := mouseTestApp(t)
	application.terminalHistory = make([]terminalview.Line, 20)
	application.mouseMode = true
	application.mouseReporting = true
	application.handleInput([]byte("\x1b[<64;10;5M"))
	if application.terminalScroll != 3 {
		t.Fatalf("scroll=%d", application.terminalScroll)
	}
}

func TestCtrlRightBracketTogglesMouseReporting(t *testing.T) {
	application := mouseTestApp(t)
	var output bytes.Buffer
	application.screenOut = &output

	application.handleInput([]byte{0x1d})
	if !application.mouseMode || !application.mouseReporting || output.String() != mouseReportingEnable {
		t.Fatalf("enabled mode=%v reporting=%v output=%q", application.mouseMode, application.mouseReporting, output.String())
	}

	output.Reset()
	application.handleInput([]byte{0x1d})
	if application.mouseMode || application.mouseReporting || output.String() != mouseReportingDisable {
		t.Fatalf("disabled mode=%v reporting=%v output=%q", application.mouseMode, application.mouseReporting, output.String())
	}
}

func TestMouseDragsTerminalScrollbar(t *testing.T) {
	application := mouseTestApp(t)
	application.terminalHistory = make([]terminalview.Line, 100)
	x := application.terminalScrollbarCol()
	bottom := application.rows - 1

	application.handleMouse(mouseEvent{Button: 0, X: x, Y: bottom})
	application.handleMouse(mouseEvent{Button: 32, X: x, Y: 2, Motion: true})
	if application.terminalScroll != len(application.terminalHistory) {
		t.Fatalf("scroll=%d", application.terminalScroll)
	}
	application.handleMouse(mouseEvent{Button: 0, X: x, Y: 2, Release: true})
	if application.mouseScrollbarDrag.Target != dragNoScrollbar {
		t.Fatalf("drag=%#v", application.mouseScrollbarDrag)
	}
}

func TestMouseWheelScrollsMonitorContent(t *testing.T) {
	application := mouseTestApp(t)
	application.monitorLayout = monitorLayoutSingle
	application.focus = focusMonitor
	application.panels.UpdateContent("system", strings.Repeat("line\n", 80), "实时", true)
	top, left, _, _ := application.monitorRegion()
	application.handleMouse(mouseEvent{Button: 64, X: left + 2, Y: top + 2})
	if application.panelScroll["system"] == 0 || application.panels.Active().ID != "system" {
		t.Fatalf("scroll=%d active=%s", application.panelScroll["system"], application.panels.Active().ID)
	}
}

func TestMouseRejectsInlineProposal(t *testing.T) {
	application := mouseTestApp(t)
	application.agent = approvalTestProvider{}
	application.atPrompt = true
	application.inlineAI = &inlineAIState{Prompt: "查看负载", AnchorX: 20, AnchorY: 1, Started: time.Now()}
	application.agentInline = application.inlineAI
	application.finishAgentTurn(agent.Turn{
		Type: "command_proposal",
		Proposal: &agent.Proposal{
			ID: "p1", Command: "uptime", Explanation: "查看负载", Risk: "read_only",
		},
	}, nil)
	top, left, width, height := application.inlineArea()
	lines := application.inlineProposalLines(width, height-1)
	buttonLine := lines[len(lines)-2]
	buttonRow := top + len(lines) - 1
	index := strings.Index(buttonLine, "[ no 拒绝 ]")
	if index < 0 {
		t.Fatalf("buttons=%q", buttonLine)
	}
	clickMouse(application, left+textWidth(buttonLine[:index]), buttonRow)
	select {
	case event := <-application.events:
		application.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("mouse rejection timed out")
	}
	if application.proposal != nil || !strings.Contains(strings.Join(application.aiLines(60), "\n"), "已拒绝命令建议：uptime") {
		t.Fatalf("proposal=%#v conversation=%q", application.proposal, strings.Join(application.aiLines(60), "\n"))
	}
}

func TestSplitMouseSequenceIsBuffered(t *testing.T) {
	application := mouseTestApp(t)
	application.handleMixedInput([]byte("\x1b[<0;92"))
	if len(application.mouseCarry) == 0 {
		t.Fatal("partial mouse sequence was not buffered")
	}
	application.handleMixedInput([]byte(";2M"))
	application.handleMixedInput([]byte("\x1b[<0;92;2m"))
	if len(application.mouseCarry) != 0 || application.focus != focusMonitor {
		t.Fatalf("carry=%q focus=%v", application.mouseCarry, application.focus)
	}
}

func mouseTestApp(t *testing.T) *App {
	t.Helper()
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.mainCols = 89
	application.contentRows = 34
	return application
}

func clickMouse(application *App, x, y int) {
	application.handleMouse(mouseEvent{Button: 0, X: x, Y: y})
	application.handleMouse(mouseEvent{Button: 0, X: x, Y: y, Release: true})
}
