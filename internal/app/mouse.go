package app

import (
	"bytes"
	"io"
	"strconv"
	"strings"
)

var sgrMousePrefix = []byte("\x1b[<")

const (
	mouseReportingEnable  = "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
	mouseReportingDisable = "\x1b[?1000l\x1b[?1002l\x1b[?1006l"
)

type scrollbarDragTarget uint8

const (
	dragNoScrollbar scrollbarDragTarget = iota
	dragTerminalScrollbar
	dragAIScrollbar
)

type scrollbarDragState struct {
	Target scrollbarDragTarget
	Grab   int
}

type mouseEvent struct {
	Button  int
	X       int
	Y       int
	Release bool
	Motion  bool
}

func (a *App) handleMixedInput(data []byte) {
	if len(a.mouseCarry) > 0 {
		data = append(append([]byte(nil), a.mouseCarry...), data...)
		a.mouseCarry = nil
	}
	for len(data) > 0 {
		start := bytes.Index(data, sgrMousePrefix)
		if start < 0 {
			a.handleKeyboardInput(data)
			return
		}
		if start > 0 {
			a.handleKeyboardInput(data[:start])
			data = data[start:]
		}
		end := mouseSequenceEnd(data)
		if end < 0 {
			if len(data) <= 64 {
				a.mouseCarry = append([]byte(nil), data...)
				return
			}
			a.handleKeyboardInput(data)
			return
		}
		sequence := data[:end+1]
		if event, ok := parseSGRMouse(sequence); ok {
			a.handleMouse(event)
		} else {
			a.handleKeyboardInput(sequence)
		}
		data = data[end+1:]
	}
}

func mouseSequenceEnd(data []byte) int {
	for index := len(sgrMousePrefix); index < len(data); index++ {
		if data[index] == 'M' || data[index] == 'm' {
			return index
		}
	}
	return -1
}

func parseSGRMouse(sequence []byte) (mouseEvent, bool) {
	if len(sequence) < 7 || !bytes.HasPrefix(sequence, sgrMousePrefix) {
		return mouseEvent{}, false
	}
	terminator := sequence[len(sequence)-1]
	if terminator != 'M' && terminator != 'm' {
		return mouseEvent{}, false
	}
	parts := strings.Split(string(sequence[len(sgrMousePrefix):len(sequence)-1]), ";")
	if len(parts) != 3 {
		return mouseEvent{}, false
	}
	values := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return mouseEvent{}, false
		}
		values[index] = value
	}
	if values[1] < 1 || values[2] < 1 {
		return mouseEvent{}, false
	}
	return mouseEvent{Button: values[0], X: values[1], Y: values[2], Release: terminator == 'm', Motion: values[0]&32 != 0}, true
}

func (a *App) handleMouse(event mouseEvent) {
	if event.Release {
		a.mouseScrollbarDrag = scrollbarDragState{}
		return
	}
	if event.Motion {
		if event.Button&3 == 0 && a.mouseScrollbarDrag.Target != dragNoScrollbar {
			a.dragScrollbarTo(event.Y)
			a.dirty = true
		}
		return
	}
	if event.Button&64 != 0 {
		a.handleMouseWheel(event)
		a.dirty = true
		return
	}
	if event.Button&3 != 0 {
		return
	}
	if a.beginScrollbarDrag(event.X, event.Y) {
		a.dirty = true
		return
	}
	a.handleLeftClick(event.X, event.Y)
	a.dirty = true
}

func (a *App) updateMouseReporting() string {
	if a.mouseMode == a.mouseReporting {
		return ""
	}
	a.mouseReporting = a.mouseMode
	if a.mouseReporting {
		return mouseReportingEnable
	}
	a.mouseCarry = nil
	a.mouseScrollbarDrag = scrollbarDragState{}
	return mouseReportingDisable
}

func (a *App) toggleMouseMode() {
	a.mouseMode = !a.mouseMode
	if sequence := a.updateMouseReporting(); sequence != "" {
		_, _ = io.WriteString(a.outputWriter(), sequence)
	}
	a.dirty = true
}

func (a *App) mouseModeHint() string {
	if a.mouseMode {
		return a.t("滚动模式 · Ctrl+] 恢复选择")
	}
	return a.t("选择模式 · Ctrl+] 开启滚动")
}

func (a *App) beginScrollbarDrag(x, y int) bool {
	terminalVisible := !a.compact || a.focus == focusTerminal || a.focus == focusApproval
	if terminalVisible && len(a.terminalHistory) > 0 && x == a.terminalScrollbarCol() && y >= 2 && y < a.rows {
		total := len(a.terminalHistory) + a.contentRows
		a.mouseScrollbarDrag = scrollbarDragState{
			Target: dragTerminalScrollbar,
			Grab:   scrollbarGrabOffset(y-2, a.contentRows, total, a.contentRows, a.terminalScroll),
		}
		a.dragScrollbarTo(y)
		return true
	}

	if a.focus == focusAIHistory {
		return false
	}
	aiTop, aiLeft, aiWidth, aiHeight := a.aiRegion()
	contentHeight := aiConversationViewport(aiHeight, true)
	if contentHeight <= 1 || x != aiLeft+aiWidth-1 || y < aiTop+1 || y >= aiTop+1+contentHeight {
		return false
	}
	total := len(a.aiLines(aiWidth - 2))
	if total <= contentHeight {
		return false
	}
	a.mouseScrollbarDrag = scrollbarDragState{
		Target: dragAIScrollbar,
		Grab:   scrollbarGrabOffset(y-(aiTop+1), contentHeight, total, contentHeight, a.aiScroll),
	}
	a.dragScrollbarTo(y)
	return true
}

func (a *App) dragScrollbarTo(y int) {
	switch a.mouseScrollbarDrag.Target {
	case dragTerminalScrollbar:
		total := len(a.terminalHistory) + a.contentRows
		a.terminalScroll = scrollbarOffsetAt(
			y-2,
			a.mouseScrollbarDrag.Grab,
			a.contentRows,
			total,
			a.contentRows,
		)
		a.focus = focusTerminal
	case dragAIScrollbar:
		top, _, width, height := a.aiRegion()
		viewport := aiConversationViewport(height, true)
		total := len(a.aiLines(width - 2))
		a.aiScroll = scrollbarOffsetAt(
			y-(top+1),
			a.mouseScrollbarDrag.Grab,
			viewport,
			total,
			viewport,
		)
		a.focus = focusAI
		a.aiExpanded = true
	}
}

func scrollbarGrabOffset(position, height, total, viewport, offset int) int {
	thumbTop, thumbSize := scrollbarThumb(height, total, viewport, offset)
	if position >= thumbTop && position < thumbTop+thumbSize {
		return position - thumbTop
	}
	return thumbSize / 2
}

func scrollbarOffsetAt(position, grab, height, total, viewport int) int {
	maximum := total - viewport
	if maximum <= 0 || height <= 0 {
		return 0
	}
	_, thumbSize := scrollbarThumb(height, total, viewport, 0)
	track := height - thumbSize
	if track <= 0 {
		return 0
	}
	thumbTop := position - grab
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop > track {
		thumbTop = track
	}
	start := (thumbTop*maximum + track/2) / track
	return maximum - start
}

func (a *App) handleLeftClick(x, y int) {
	if a.handleScrollbarClick(x, y) {
		return
	}
	if a.handleCommandExpandClick(x, y) {
		return
	}
	if a.handleApprovalClick(x, y) {
		return
	}
	if a.handleInlineAIClick(x, y) {
		return
	}
	if a.compact {
		if a.focus == focusAI && a.handleAICommandExpandClick(x, y) {
			return
		}
		a.handleCompactClick(x, y)
		return
	}
	if x <= a.terminalScrollbarCol() && y >= 2 && y < a.rows {
		if a.proposal == nil {
			a.focus = focusTerminal
		}
		return
	}
	monitorTop, monitorLeft, monitorWidth, monitorHeight := a.monitorRegion()
	if pointInRegion(x, y, monitorTop, monitorLeft, monitorWidth, monitorHeight) {
		if a.focus == focusMonitorSlot && y > monitorTop {
			a.handleMonitorSlotClick(y)
			return
		}
		if y == monitorTop {
			if x >= a.monitorLayoutHeaderStart() {
				a.cycleMonitorLayout()
				a.focus = focusMonitor
				return
			}
			if id, add, ok := a.monitorHeaderTarget(x); ok {
				if add {
					a.focus = focusPanelMenu
					a.menuStatus = ""
				} else {
					a.panels.Activate(id)
					a.focus = focusMonitor
				}
				return
			}
		}
		if slot, ok := a.monitorSlotButtonTarget(x, y); ok {
			a.openMonitorSlotPicker(slot)
			return
		}
		if item := a.monitorTileAt(x, y); item != nil {
			a.panels.Activate(item.ID)
		}
		a.focus = focusMonitor
		return
	}
	aiTop, aiLeft, aiWidth, aiHeight := a.aiRegion()
	if pointInRegion(x, y, aiTop, aiLeft, aiWidth, aiHeight) {
		if a.handleAICommandExpandClick(x, y) {
			return
		}
		if y == aiTop && x >= a.aiHistoryHeaderStart() {
			if a.focus == focusAIHistory {
				a.focus = focusAI
			} else {
				a.openAIHistory()
			}
			return
		}
		if a.focus == focusAIHistory {
			a.handleAIHistoryClick(y)
			return
		}
		a.openAIConversation()
	}
}

func (a *App) handleInlineAIClick(x, y int) bool {
	if a.compact && a.focus != focusTerminal && a.focus != focusApproval {
		return false
	}
	if x < 1 || x > a.mainCols || y < 2 || y >= a.rows {
		return false
	}
	for _, layout := range a.buildTerminalLayout().Blocks {
		if x >= layout.Left && x < layout.Left+layout.Width && y >= layout.Top && y <= layout.Top+layout.Height {
			a.openAIConversation()
			return true
		}
	}
	return false
}

func (a *App) handleCompactClick(x, y int) {
	if x < 1 || x > a.cols || y < 2 || y >= a.rows {
		return
	}
	switch a.focus {
	case focusAI, focusAIHistory:
		aiTop, _, _, _ := a.aiRegion()
		if y == aiTop && x >= a.aiHistoryHeaderStart() {
			if a.focus == focusAIHistory {
				a.focus = focusAI
			} else {
				a.openAIHistory()
			}
			return
		}
		if a.focus == focusAIHistory {
			a.handleAIHistoryClick(y)
			return
		}
		a.aiExpanded = true
	case focusMonitorSlot:
		a.handleMonitorSlotClick(y)
	case focusMonitor, focusPanelMenu, focusCustomPanel:
		monitorTop, _, _, _ := a.monitorRegion()
		if y == monitorTop {
			if x >= a.monitorLayoutHeaderStart() {
				a.cycleMonitorLayout()
				a.focus = focusMonitor
				return
			}
			if id, add, ok := a.monitorHeaderTarget(x); ok {
				if add {
					a.focus = focusPanelMenu
				} else {
					a.panels.Activate(id)
					a.focus = focusMonitor
				}
			}
		}
		if slot, ok := a.monitorSlotButtonTarget(x, y); ok {
			a.openMonitorSlotPicker(slot)
			return
		}
		if item := a.monitorTileAt(x, y); item != nil {
			a.panels.Activate(item.ID)
			a.focus = focusMonitor
		}
	default:
		a.focus = focusTerminal
	}
}

func (a *App) handleAIHistoryClick(y int) {
	top, _, _, height := a.aiRegion()
	available := height - 2
	if available <= 0 || y < top+1 || y >= top+1+available {
		return
	}
	start, end := a.historyPickerWindow(available)
	index := start + y - (top + 1)
	if index < start || index >= end {
		return
	}
	a.historySelection = index
	a.selectHistoryConversation(index)
}

func (a *App) handleMonitorSlotClick(y int) {
	top, _, _, height := a.monitorRegion()
	if y <= top || y >= top+height {
		return
	}
	lineIndex := y - (top + 1)
	panelIndex := lineIndex - 2
	if panelIndex >= 0 {
		a.assignMonitorSlot(panelIndex)
	}
}

func (a *App) handleMouseWheel(event mouseEvent) {
	if a.compact {
		switch a.focus {
		case focusTerminal, focusApproval:
			a.scrollTerminal(event.Button&1 == 0)
		case focusAI:
			a.scrollAI(event.Button&1 == 0)
		case focusAIHistory:
			if event.Button&1 == 0 {
				a.moveHistorySelection(-1)
			} else {
				a.moveHistorySelection(1)
			}
		case focusMonitor:
			a.scrollMonitorAt(event.X, event.Y, event.Button&1 == 0)
		case focusPanelMenu, focusCustomPanel:
			a.scrollMonitorTabs(event.Button&1 == 0)
		}
		return
	}
	if event.X >= 1 && event.X <= a.terminalScrollbarCol() && event.Y >= 2 && event.Y < a.rows {
		a.scrollTerminal(event.Button&1 == 0)
		return
	}
	monitorTop, monitorLeft, monitorWidth, monitorHeight := a.monitorRegion()
	if pointInRegion(event.X, event.Y, monitorTop, monitorLeft, monitorWidth, monitorHeight) {
		if a.focus == focusMonitorSlot {
			return
		}
		if event.Y == monitorTop {
			a.scrollMonitorTabs(event.Button&1 == 0)
		} else {
			a.scrollMonitorAt(event.X, event.Y, event.Button&1 == 0)
		}
		return
	}
	aiTop, aiLeft, aiWidth, aiHeight := a.aiRegion()
	if pointInRegion(event.X, event.Y, aiTop, aiLeft, aiWidth, aiHeight) {
		if a.focus == focusAIHistory {
			if event.Button&1 == 0 {
				a.moveHistorySelection(-1)
			} else {
				a.moveHistorySelection(1)
			}
		} else {
			a.scrollAI(event.Button&1 == 0)
		}
	}
}

func (a *App) scrollMonitorAt(x, y int, up bool) {
	top, left, width, height := a.monitorRegion()
	if y == top {
		a.scrollMonitorTabs(up)
		return
	}
	item := a.monitorTileAt(x, y)
	viewport := height - 1
	lineWidth := width - 2
	if item != nil {
		for _, tile := range a.monitorTiles(top+1, left, width, height-1) {
			if tile.Panel.ID == item.ID {
				viewport = tile.Height - 2
				lineWidth = tile.Width - 4
				break
			}
		}
		a.panels.Activate(item.ID)
	} else {
		item = a.panels.Active()
	}
	if item == nil || viewport <= 0 || lineWidth <= 0 {
		return
	}
	maximum := len(a.panelLines(item, lineWidth)) - viewport
	if maximum < 0 {
		maximum = 0
	}
	offset := a.panelScroll[item.ID]
	if up {
		offset += 3
		if offset > maximum {
			offset = maximum
		}
	} else {
		offset -= 3
		if offset < 0 {
			offset = 0
		}
	}
	a.panelScroll[item.ID] = offset
	a.focus = focusMonitor
}

func (a *App) clampedPanelScroll(panelID string, total, viewport int) int {
	if panelID == "" || viewport <= 0 {
		return 0
	}
	maximum := total - viewport
	if maximum < 0 {
		maximum = 0
	}
	offset := a.panelScroll[panelID]
	if offset > maximum {
		offset = maximum
		a.panelScroll[panelID] = offset
	}
	return offset
}

func (a *App) handleScrollbarClick(x, y int) bool {
	terminalVisible := !a.compact || a.focus == focusTerminal || a.focus == focusApproval
	if terminalVisible && len(a.terminalHistory) > 0 && x == a.terminalScrollbarCol() && y >= 2 && y < a.rows {
		a.jumpTerminalScroll(y)
		return true
	}
	aiTop, aiLeft, aiWidth, aiHeight := a.aiRegion()
	if a.focus == focusAIHistory {
		return false
	}
	// The input box is always reserved, so clicking the scrollbar does not alter
	// the conversation viewport or make the scroll position jump.
	contentHeight := aiConversationViewport(aiHeight, true)
	if contentHeight <= 1 || x != aiLeft+aiWidth-1 || y < aiTop+1 || y >= aiTop+aiHeight {
		return false
	}
	if y >= aiTop+1+contentHeight {
		return false
	}
	total := len(a.aiLines(aiWidth - 2))
	maximum := total - contentHeight
	if maximum <= 0 {
		return false
	}
	relative := y - (aiTop + 1)
	start := maximum * relative / (contentHeight - 1)
	a.aiScroll = maximum - start
	a.focus = focusAI
	a.aiExpanded = true
	return true
}

func (a *App) scrollMonitorTabs(up bool) {
	if up {
		a.panels.Previous()
	} else {
		a.panels.Next()
	}
	a.focus = focusMonitor
}

func (a *App) scrollAI(up bool) {
	_, _, width, height := a.aiRegion()
	contentHeight := aiConversationViewport(height, true)
	maximum := len(a.aiLines(width-2)) - contentHeight
	if maximum < 0 {
		maximum = 0
	}
	if up {
		a.aiScroll += 3
		if a.aiScroll > maximum {
			a.aiScroll = maximum
		}
	} else {
		a.aiScroll -= 3
		if a.aiScroll < 0 {
			a.aiScroll = 0
		}
	}
	a.focus = focusAI
	a.aiExpanded = true
}

func (a *App) monitorHeaderTarget(x int) (id string, add bool, ok bool) {
	_, left, width, _ := a.monitorRegion()
	tabWidth := a.monitorTabAreaWidth()
	tabStart := left + textWidth(a.t("监控 "))
	if x < tabStart || x >= tabStart+tabWidth || x >= left+width {
		return "", false, false
	}
	cursor := tabStart
	panels := a.panels.Panels()
	for index, item := range panels {
		label := item.Title
		if item.Unread {
			label += "*"
		}
		if index == a.panels.ActiveIndex() {
			label = "[" + label + "]"
		} else {
			label = " " + label + " "
		}
		end := cursor + textWidth(label)
		if x >= cursor && x < end {
			return item.ID, false, true
		}
		cursor = end
		if index < len(panels)-1 {
			cursor++
		}
	}
	cursor += 2
	if x == cursor {
		return "", true, true
	}
	return "", false, false
}

func (a *App) handleApprovalClick(x, y int) bool {
	if a.proposal == nil || a.agentBusy || a.approvalStatus != "" {
		return false
	}
	if a.inlineApprovalAvailable() {
		top, left, width, height := a.inlineArea()
		lines := a.inlineProposalLines(width, height-1)
		if len(lines) >= 2 {
			buttonLine := lines[len(lines)-2]
			buttonRow := top + len(lines) - 1
			if choice, ok := a.approvalButtonAt(x, y, left, buttonRow, buttonLine); ok {
				a.applyMouseApproval(choice)
				return true
			}
		}
		return false
	}
	top, left, _, height := a.approvalGeometry()
	buttonRow := top + height - 3
	if choice, ok := a.approvalButtonAt(x, y, left+2, buttonRow, a.approvalButtonsText()); ok {
		a.applyMouseApproval(choice)
		return true
	}
	return false
}

func (a *App) applyMouseApproval(approve bool) {
	a.focus = focusApproval
	a.approvalInput = nil
	if approve {
		a.approveProposal()
	} else {
		a.rejectProposal()
	}
}

func (a *App) approvalButtonAt(x, y, left, row int, line string) (bool, bool) {
	if y != row {
		return false, false
	}
	if textHit(x, left, line, a.t("[ yes 执行 ]")) {
		return true, true
	}
	if textHit(x, left, line, a.t("[ no 拒绝 ]")) {
		return false, true
	}
	return false, false
}

func textHit(x, left int, line, target string) bool {
	index := strings.Index(line, target)
	if index < 0 {
		return false
	}
	start := left + textWidth(line[:index])
	return x >= start && x < start+textWidth(target)
}

func pointInRegion(x, y, top, left, width, height int) bool {
	return x >= left && x < left+width && y >= top && y < top+height
}
