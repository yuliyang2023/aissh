package app

import (
	"fmt"
	"io"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

func (a *App) render() {
	if a.zmodem != nil {
		return
	}
	if a.nativeSelection && !a.selectionNeedsRender {
		return
	}
	var output strings.Builder
	terminalCursorRow, terminalCursorCol := 0, 0
	// DEC mode 2026 lets supporting terminals present the assembled frame atomically.
	// Unsupported terminals safely ignore it.
	output.WriteString("\x1b[?2026h\x1b[?25l")
	header := fmt.Sprintf(" aissh  %s", a.parsed.Target)
	if a.agent.Configured() {
		header += "  AI:" + a.agent.Name()
	} else {
		header += "  AI:" + a.agent.Name() + a.t("不可用")
	}
	output.WriteString(positionedLine(1, 1, header, a.cols, "\x1b[1;38;5;45m"))

	workspaceOnly := a.compact && (a.focus == focusAI || a.focus == focusAIHistory || a.focus == focusMonitor || a.focus == focusMonitorSlot || a.focus == focusPanelMenu || a.focus == focusCustomPanel)
	if !workspaceOnly {
		terminalOutput, cursorRow, cursorCol := a.renderTerminalWorkspace()
		terminalCursorRow, terminalCursorCol = cursorRow, cursorCol
		output.WriteString(terminalOutput)
		if a.aiLineCapture && a.focus == focusTerminal && a.virtual != nil {
			cursor := a.virtual.Cursor()
			available := a.mainCols - cursor.X
			output.WriteString(positionedLine(terminalCursorRow, 1+cursor.X, string(a.lineInput), available, "\x1b[38;5;213m"))
			delete(a.terminalRows, terminalCursorRow)
		}
		if a.proposal != nil && !a.inlineApprovalAvailable() {
			output.WriteString(a.renderApprovalCard())
			top, _, _, height := a.approvalGeometry()
			for row := top; row < top+height; row++ {
				delete(a.terminalRows, row)
			}
		}
	} else {
		// The compact right-hand workspace replaces the SSH pane. Force its rows to
		// be restored when the user returns to the terminal.
		a.terminalRows = nil
	}
	if a.compact {
		if workspaceOnly {
			if a.focus == focusAI || a.focus == focusAIHistory {
				output.WriteString(a.renderAIWorkspace(2, 1, a.cols, a.contentRows))
			} else {
				output.WriteString(a.renderMonitorWorkspace(2, 1, a.cols, a.contentRows))
			}
		}
	} else {
		borderCol := a.mainCols + 2
		for row := 2; row < a.rows; row++ {
			output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[38;5;238m│", row, borderCol))
		}
		output.WriteString(a.renderSidebar(2, borderCol+1, a.cols-borderCol, a.contentRows))
	}

	output.WriteString(positionedLine(a.rows, 1, a.inputStatus(), a.cols, "\x1b[38;5;245m"))
	a.captureSelectionCanvas(output.String())
	a.renderTextSelection(&output)
	switch {
	case a.focus == focusTerminal && a.terminalScroll == 0 && a.proposal == nil && !workspaceOnly && a.virtual != nil && (a.virtual.CursorVisible() || a.atPrompt):
		cursorCol := terminalCursorCol
		if a.aiLineCapture {
			a.clampCapturedLineCursor()
			cursorCol += textWidth(string(a.lineInput[:a.lineCursor]))
		}
		if cursorCol > a.mainCols {
			cursorCol = a.mainCols
		}
		output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", terminalCursorRow, cursorCol))
	case a.focus == focusApproval && a.proposal != nil:
		if row, col, ok := a.inlineApprovalCursor(); ok {
			output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", row, col))
		} else {
			top, left, width, height := a.approvalGeometry()
			cursorCol := left + 2 + textWidth(a.t("输入 yes/no > ")+string(a.approvalInput))
			if cursorCol >= left+width-1 {
				cursorCol = left + width - 2
			}
			output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", top+height-2, cursorCol))
		}
	case a.focus == focusAI:
		top, left, width, height := a.aiRegion()
		if cursorRow, cursorCol, ok := a.aiEditorCursorPosition(top, left, width, height); ok {
			output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", cursorRow, cursorCol))
		}
	case a.focus == focusCustomPanel:
		top, left, _, _ := a.monitorRegion()
		cursorCol := left + 3 + textWidth(string(a.editor))
		if cursorCol > a.cols {
			cursorCol = a.cols
		}
		output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", top+5, cursorCol))
	}
	output.WriteString("\x1b[?2026l")
	_, _ = io.WriteString(a.outputWriter(), output.String())
	if a.nativeSelection {
		a.selectionNeedsRender = false
	}
}

func (a *App) renderSidebar(top, left, width, height int) string {
	monitorHeight, aiHeight := a.sidebarHeights(height)
	var output strings.Builder
	output.WriteString(a.renderMonitorWorkspace(top, left, width, monitorHeight))
	separatorRow := top + monitorHeight
	output.WriteString(positionedLine(separatorRow, left, strings.Repeat("┄", width), width, "\x1b[38;5;238m\x1b[48;5;234m"))
	output.WriteString(a.renderAIWorkspace(separatorRow+1, left, width, aiHeight))
	return output.String()
}

func (a *App) sidebarHeights(height int) (int, int) {
	aiHeight := 2
	if a.aiExpanded || a.focus == focusAI || a.focus == focusAIHistory || (a.agentBusy && a.inlineAI == nil) {
		aiHeight = height * 2 / 5
		if aiHeight < 7 {
			aiHeight = 7
		}
	}
	if aiHeight > height-6 {
		aiHeight = height - 6
	}
	if aiHeight < 2 {
		aiHeight = 2
	}
	monitorHeight := height - aiHeight - 1
	if monitorHeight < 4 {
		monitorHeight = 4
		aiHeight = height - monitorHeight - 1
	}
	return monitorHeight, aiHeight
}

func (a *App) monitorRegion() (top, left, width, height int) {
	if a.compact {
		return 2, 1, a.cols, a.contentRows
	}
	monitorHeight, _ := a.sidebarHeights(a.contentRows)
	return 2, a.mainCols + 3, a.cols - a.mainCols - 2, monitorHeight
}

func (a *App) aiRegion() (top, left, width, height int) {
	if a.compact {
		return 2, 1, a.cols, a.contentRows
	}
	monitorHeight, aiHeight := a.sidebarHeights(a.contentRows)
	return 3 + monitorHeight, a.mainCols + 3, a.cols - a.mainCols - 2, aiHeight
}

func (a *App) renderMonitorWorkspace(top, left, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var output strings.Builder
	output.WriteString(positionedLine(top, left, a.monitorHeaderLine(width), width, "\x1b[1;38;5;81m"))
	contentTop := top + 1
	contentHeight := height - 1
	if contentHeight <= 0 {
		return output.String()
	}
	var lines []string
	scrollID := ""
	switch a.focus {
	case focusMonitorSlot:
		lines = a.monitorSlotPickerLines(width - 2)
	case focusPanelMenu:
		lines = a.panelMenuLines()
	case focusCustomPanel:
		lines = []string{
			a.t("新增自定义监控"),
			"",
			a.menuStatus,
			"",
			"> " + string(a.editor),
			"",
			a.t("监控命令必须是只读命令。"),
		}
	default:
		if tiles := a.monitorTiles(contentTop, left, width, contentHeight); len(tiles) > 0 {
			// Grid pages are not always full, and their geometry can change when the
			// sidebar or layout mode changes. Clear the complete monitor canvas so an
			// unused tile cannot retain colors and text from an older frame.
			for row := 0; row < contentHeight; row++ {
				output.WriteString(positionedLine(contentTop+row, left, "", width, "\x1b[0m"))
			}
			output.WriteString(a.renderMonitorTiles(tiles))
			return output.String()
		}
		lines = a.activePanelLines(width - 2)
		if active := a.panels.Active(); active != nil {
			scrollID = active.ID
		}
	}
	totalLines := len(lines)
	offset := a.clampedPanelScroll(scrollID, totalLines, contentHeight)
	if scrollID == "" {
		lines = limitLines(lines, contentHeight)
	} else {
		lines = fitTailOffset(lines, contentHeight, offset)
	}
	for index := 0; index < contentHeight; index++ {
		value := ""
		if index < len(lines) {
			value = lines[index]
		}
		output.WriteString(positionedLine(contentTop+index, left, " "+value, width, "\x1b[0m"))
	}
	if scrollID != "" {
		output.WriteString(renderScrollbar(contentTop, left+width-1, contentHeight, totalLines, contentHeight, offset))
	}
	return output.String()
}

func (a *App) renderAIWorkspace(top, left, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var output strings.Builder
	output.WriteString(positionedLine(top, left, a.aiHeaderLine(width), width, aiHeaderStyle))
	if height <= 1 {
		return output.String()
	}
	if a.focus == focusAIHistory {
		contentHeight := height - 1
		available := contentHeight - 1
		if available < 0 {
			available = 0
		}
		rows := a.aiHistoryRows(width-2, available)
		for index := 0; index < available; index++ {
			line := aiDisplayLine{Style: aiCanvasStyle}
			if index < len(rows) {
				line = rows[index]
			}
			output.WriteString(positionedLine(top+1+index, left, " "+line.Text, width, line.Style))
		}
		if contentHeight > 0 {
			output.WriteString(positionedLine(top+height-1, left, " "+a.t("↑/↓ 选择 · Enter 打开 · N 新建 · Esc 返回"), width, aiMutedStyle))
		}
		return output.String()
	}
	if height == 2 && a.focus != focusAI {
		output.WriteString(positionedLine(top+1, left, " "+a.compactAIHint(a.aiExpanded), width, aiMutedStyle))
		return output.String()
	}

	contentWidth := width - 2
	conversationHeight := aiConversationViewport(height, a.focus == focusAI)
	allLines := a.aiDisplayLines(contentWidth)
	lines := fitTailDisplayOffset(allLines, conversationHeight, a.aiScroll)
	for index := 0; index < conversationHeight; index++ {
		line := aiDisplayLine{Style: aiCanvasStyle}
		if index < len(lines) {
			line = lines[index]
		}
		output.WriteString(positionedLine(top+1+index, left, " "+line.Text, width, line.Style))
	}
	output.WriteString(renderScrollbar(top+1, left+width-1, conversationHeight, len(allLines), conversationHeight, a.aiScroll))
	bodyRows := aiInputBodyRows(height)
	if bodyRows > 0 {
		inputTop := top + 1 + conversationHeight
		titleStyle, bodyStyle := aiMutedStyle, aiCanvasStyle
		if a.focus == focusAI {
			titleStyle, bodyStyle = aiInputTopStyle, aiInputStyle
		}
		output.WriteString(positionedLine(inputTop, left, " "+a.aiInputTitle(a.agentBusy, contentWidth), width, titleStyle))
		layout := a.buildAIEditorLayout(width, bodyRows)
		for index := 0; index < bodyRows; index++ {
			line := ""
			if index < len(layout.Lines) {
				line = layout.Lines[index]
			}
			output.WriteString(positionedLine(inputTop+1+index, left, line, width, bodyStyle))
		}
	}
	return output.String()
}

func (a *App) hasAIConversation() bool {
	for _, message := range a.chat {
		if message.Role == "user" {
			return true
		}
	}
	return false
}

func (a *App) tabBar(width int) string {
	var items []string
	for index, item := range a.panels.Panels() {
		label := item.Title
		if item.Unread {
			label += "*"
		}
		if index == a.panels.ActiveIndex() {
			items = append(items, "["+label+"]")
		} else {
			items = append(items, " "+label+" ")
		}
	}
	return terminalview.ClipPad(strings.Join(items, " ")+"  +", width)
}

func (a *App) activePanelLines(width int) []string {
	return a.panelLines(a.panels.Active(), width)
}

func (a *App) monitorLines(item *panel.Panel, width int) []string {
	var lines []string
	origin := map[panel.Origin]string{panel.OriginAI: a.t("AI创建"), panel.OriginUser: a.t("用户创建"), panel.OriginSystem: a.t("内置")}[item.Origin]
	status := a.t(item.Status)
	if item.LastUpdated.IsZero() {
		status = a.t("等待连接")
	} else {
		status = fmt.Sprintf("%s · %s", status, item.LastUpdated.Format("15:04:05"))
	}
	lines = append(lines, fmt.Sprintf("%s · %s", origin, status), "")
	if item.LastError != "" {
		lines = append(lines, a.t("采集错误:"), item.LastError, "", a.t("主 SSH 建立后会自动重试。"))
		return terminalview.Wrap(strings.Join(lines, "\n"), width)
	}
	content := trigger.StripANSI(item.Content)
	if strings.TrimSpace(content) == "" {
		content = a.t("等待监控数据……")
	}
	lines = append(lines, terminalview.Wrap(content, width)...)
	return lines
}

func (a *App) panelMenuLines() []string {
	return []string{
		a.t("新增面板"),
		"",
		a.t("1  系统概览"),
		a.t("2  进程排行"),
		a.t("3  磁盘使用"),
		a.t("4  网络状态"),
		a.t("5  服务状态"),
		a.t("6  自定义只读命令"),
		"",
		a.t("Esc 取消"),
	}
}

var inlineSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (a *App) renderInlineAI() string {
	if a.inlineAI == nil {
		return ""
	}
	return a.renderInlineBlock(a.inlineAI)
}

func (a *App) renderInlineBlock(state *inlineAIState) string {
	layout, ok := a.inlineLayoutFor(state)
	if !ok {
		return ""
	}
	return a.renderInlineLayout(layout)
}

func (a *App) inlineArea() (top, left, width, height int) {
	if a.inlineAI == nil {
		return 0, 0, 0, 0
	}
	layout, ok := a.inlineLayoutFor(a.inlineAI)
	if !ok {
		return 0, 0, 0, 0
	}
	top, left, width = layout.Top, layout.Left, layout.Width
	height = layout.Height + 1
	if top+height > a.screenRows() {
		height = a.screenRows() - top
	}
	return top, left, width, height
}

func (a *App) inlineConversationLines(width, available int) []string {
	return a.inlineConversationLinesFor(a.inlineAI, width, available)
}

func (a *App) inlineConversationLinesFor(state *inlineAIState, width, available int) []string {
	if state == nil || available <= 0 || width <= 0 {
		return nil
	}
	if state.Proposal != nil {
		return a.inlineProposalLinesFor(state, width, available)
	}
	if !state.Done {
		frame := inlineSpinnerFrames[state.Spinner%len(inlineSpinnerFrames)]
		status := firstText(a.t(state.Status), a.t("本机 %s 正在分析", a.agent.Name()))
		elapsed := time.Since(state.Started).Round(time.Second)
		lines := []string{fmt.Sprintf("AI %s %s · %s", frame, status, elapsed)}
		if strings.TrimSpace(state.Response) != "" && available > 1 {
			streamed := terminalview.Wrap("AI: "+state.Response, width)
			lines = append(lines, limitLines(streamed, available-1)...)
		}
		return limitLines(lines, available)
	}
	response := strings.TrimSpace(state.Response)
	if response == "" {
		response = a.t("处理完成。")
	}
	prefix := "AI: "
	if state.Error != "" {
		prefix = a.t("AI 错误: ")
	}
	lines := terminalview.Wrap(prefix+response, width)
	if len(lines) > available {
		lines = append([]string(nil), lines[:available]...)
		if available > 1 {
			lines[available-1] = terminalview.ClipPad(a.t("… 点击查看完整对话"), width)
		}
	}
	return lines
}

func (a *App) inlineProposalLines(width, available int) []string {
	return a.inlineProposalLinesFor(a.inlineAI, width, available)
}

func (a *App) inlineProposalLinesFor(state *inlineAIState, width, available int) []string {
	if state == nil || state.Proposal == nil || available < 3 {
		return nil
	}
	proposal := state.Proposal
	activeApproval := state == a.inlineAI && a.proposal != nil && state.ApprovalResult == ""
	footerLines := 1
	if activeApproval {
		footerLines = 2
	}
	explanation := firstText(proposal.Explanation, state.Response, proposal.ExpectedEffect)
	bodySlots := available - 1 - footerLines
	reserveExplanation := 0
	if explanation != "" && bodySlots >= 2 {
		reserveExplanation = 1
	}
	commandSlots := bodySlots - reserveExplanation
	if commandSlots < 1 {
		commandSlots = 1
	}
	if !state.CommandExpanded && commandSlots > 7 {
		commandSlots = 7
	}
	command := a.formatCommandForDisplay(proposal.Command, width, commandSlots)
	if state.CommandExpanded {
		command = a.formatExpandedCommandForDisplay(proposal.Command, width, commandSlots)
	}
	lines := []string{a.t("AI 建议命令  [%s]  %d 字符", a.riskDisplay(proposal.Risk), commandCharacterCount(proposal.Command))}
	lines = append(lines, command.Lines...)
	explanationSlots := available - len(lines) - footerLines
	if explanationSlots > 0 {
		if explanation != "" {
			if explanationSlots > 2 {
				explanationSlots = 2
			}
			lines = append(lines, limitLines(terminalview.Wrap(a.t("说明  ")+explanation, width), explanationSlots)...)
		}
	}
	if activeApproval {
		lines = append(lines, firstText(a.approvalStatus, a.approvalButtonsText()), "> "+string(a.approvalInput))
	} else {
		lines = append(lines, firstText(state.ApprovalResult, a.t("命令建议已结束")))
	}
	return limitLines(lines, available)
}

func (a *App) inlineApprovalAvailable() bool {
	if a.inlineAI == nil || a.proposal == nil {
		return false
	}
	if a.inlineAI.AnchorY < 0 || a.inlineAI.AnchorY >= a.contentRows {
		return false
	}
	layout, ok := a.inlineLayoutFor(a.inlineAI)
	if !ok || layout.Top < 2 || layout.Top+layout.Height >= a.screenRows() {
		return false
	}
	return layout.Width >= 20 && a.contentRows >= 5 && a.inlineAI.Proposal != nil && a.inlineAI.ApprovalResult == ""
}

func (a *App) inlineApprovalCursor() (int, int, bool) {
	if !a.inlineApprovalAvailable() {
		return 0, 0, false
	}
	top, left, width, height := a.inlineArea()
	lines := a.inlineProposalLines(width, height-1)
	if len(lines) == 0 {
		return 0, 0, false
	}
	row := top + len(lines)
	col := left + textWidth(lines[len(lines)-1])
	if col >= left+width {
		col = left + width - 1
	}
	return row, col, true
}

func (a *App) renderApprovalCard() string {
	if a.proposal == nil {
		return ""
	}
	top, left, width, height := a.approvalGeometry()
	inner := a.approvalCardInner(width, height)

	highRisk := isReviewRisk(a.proposal.Risk)
	borderStyle := "\x1b[1;38;5;45m\x1b[48;5;235m"
	headerStyle := borderStyle
	commandStyle := "\x1b[1;38;5;220m\x1b[48;5;235m"
	if highRisk {
		borderStyle = "\x1b[1;38;5;203m\x1b[48;5;52m"
		headerStyle = "\x1b[1;38;5;255m\x1b[48;5;124m"
		commandStyle = "\x1b[1;38;5;203m\x1b[48;5;235m"
	}
	var output strings.Builder
	output.WriteString(positionedLine(top, left, "╭"+strings.Repeat("─", width-2)+"╮", width, borderStyle))
	for index, line := range inner {
		style := "\x1b[38;5;252m\x1b[48;5;235m"
		if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "  ↳ ") {
			style = commandStyle
		} else if strings.HasPrefix(line, "  …") {
			if highRisk {
				style = commandStyle
			} else {
				style = "\x1b[38;5;214m\x1b[48;5;235m"
			}
		} else if index == 0 {
			style = headerStyle
		} else if index == len(inner)-1 {
			style = "\x1b[1;38;5;118m\x1b[48;5;235m"
		} else if index == len(inner)-2 && a.approvalStatus != "" {
			style = "\x1b[38;5;214m\x1b[48;5;235m"
		}
		output.WriteString(positionedLine(top+1+index, left, framedLine(line, width), width, style))
	}
	output.WriteString(positionedLine(top+height-1, left, "╰"+strings.Repeat("─", width-2)+"╯", width, borderStyle))
	return output.String()
}

func (a *App) approvalCardInner(width, height int) []string {
	innerWidth := width - 4
	inner := make([]string, height-2)
	inner[0] = a.t("AI 命令确认    %s    %d 字符", a.riskDisplay(a.proposal.Risk), commandCharacterCount(a.proposal.Command))
	writeAt := 1
	explanation := firstText(a.proposal.Explanation, a.proposal.ExpectedEffect, a.t("AI 建议执行以下命令"))
	bodyEnd := len(inner) - 2
	reserve := 0
	if explanation != "" && bodyEnd-writeAt >= 2 {
		reserve++
	}
	if a.proposal.ExpectedEffect != "" && a.proposal.ExpectedEffect != explanation && bodyEnd-writeAt >= 3 {
		reserve++
	}
	commandSlots := bodyEnd - writeAt - reserve
	if commandSlots < 1 {
		commandSlots = 1
	}
	command := a.formatCommandForDisplay(a.proposal.Command, innerWidth, commandSlots)
	if a.approvalCommandExpanded {
		command = a.formatExpandedCommandForDisplay(a.proposal.Command, innerWidth, commandSlots)
	}
	for _, line := range command.Lines {
		if writeAt >= bodyEnd {
			break
		}
		inner[writeAt] = line
		writeAt++
	}
	if explanation != "" && writeAt < bodyEnd {
		inner[writeAt] = terminalview.ClipPad(a.t("说明  ")+explanation, innerWidth)
		writeAt++
	}
	if a.proposal.ExpectedEffect != "" && a.proposal.ExpectedEffect != explanation && writeAt < bodyEnd {
		inner[writeAt] = terminalview.ClipPad(a.t("预期  ")+a.proposal.ExpectedEffect, innerWidth)
	}
	status := firstText(a.approvalStatus, a.approvalButtonsText())
	inner[len(inner)-2] = status
	inner[len(inner)-1] = a.t("输入 yes/no > ") + string(a.approvalInput)
	return inner
}

func (a *App) approvalGeometry() (top, left, width, height int) {
	width = a.mainCols - 4
	if width < 34 {
		width = a.mainCols - 2
	}
	if width < 20 {
		width = a.mainCols
	}
	left = (a.mainCols-width)/2 + 1
	height = 12
	if a.approvalCommandExpanded {
		height = a.contentRows
	}
	if height > a.contentRows {
		height = a.contentRows
	}
	if height < 6 {
		height = 6
	}
	top = 2 + a.contentRows - height
	return top, left, width, height
}

func framedLine(value string, width int) string {
	if width < 4 {
		return terminalview.ClipPad(value, width)
	}
	return "│ " + terminalview.ClipPad(value, width-4) + " │"
}

func (a *App) approvalButtonsText() string {
	return a.t("[ yes 执行 ]  [ no 拒绝 ]")
}

func limitLines(lines []string, limit int) []string {
	if len(lines) <= limit {
		return lines
	}
	return lines[:limit]
}

func positionedLine(row, col int, value string, width int, style string) string {
	return fmt.Sprintf("\x1b[%d;%dH%s%s\x1b[0m", row, col, style, terminalview.ClipPad(value, width))
}

func fitTailOffset(lines []string, height, offset int) []string {
	if height <= 0 || len(lines) == 0 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	end := len(lines) - offset
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	return lines[start:end]
}

func formatAge(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return time.Since(value).Round(time.Second).String()
}

func textWidth(value string) int {
	width := 0
	for _, r := range value {
		width += terminalview.CellWidth(r)
	}
	return width
}
