package app

func (a *App) expandCommandShortcut() bool {
	if a.proposal != nil {
		if a.inlineApprovalAvailable() {
			if a.expandInlineCommand(a.inlineAI, true) {
				return true
			}
		} else if a.expandApprovalCardCommand() {
			return true
		}
	}
	model := a.buildTerminalLayout()
	for index := len(model.Blocks) - 1; index >= 0; index-- {
		layout := model.Blocks[index]
		if layout.State == a.inlineAI && a.proposal != nil {
			continue
		}
		if a.expandInlineLayoutCommand(layout, true) {
			return true
		}
	}
	if a.focus == focusAI {
		return a.expandLatestAIConversationCommand()
	}
	return false
}

func (a *App) expandApprovalCardCommand() bool {
	if a.proposal == nil || a.approvalCommandExpanded {
		return false
	}
	_, _, width, height := a.approvalGeometry()
	for _, line := range a.approvalCardInner(width, height) {
		if a.commandFoldLine(line) {
			a.approvalCommandExpanded = true
			a.terminalRows = nil
			a.terminalScroll = 0
			a.dirty = true
			return true
		}
	}
	return false
}

func (a *App) expandInlineCommand(state *inlineAIState, requireVisible bool) bool {
	if state == nil || state.CommandExpanded {
		return false
	}
	layout, ok := a.inlineLayoutFor(state)
	if !ok {
		return false
	}
	return a.expandInlineLayoutCommand(layout, requireVisible)
}

func (a *App) expandInlineLayoutCommand(layout inlineBlockLayout, requireVisible bool) bool {
	state := layout.State
	if state == nil || state.CommandExpanded || state.Proposal == nil {
		return false
	}
	for index, line := range layout.Lines {
		if !a.commandFoldLine(line) {
			continue
		}
		row := layout.Top + 1 + index
		if requireVisible && (row < 2 || row >= a.screenRows()) {
			continue
		}
		state.CommandExpanded = true
		a.terminalRows = nil
		a.terminalScroll = 0
		a.dirty = true
		return true
	}
	return false
}

func (a *App) expandLatestAIConversationCommand() bool {
	_, _, width, _ := a.aiRegion()
	commandWidth := width - 6
	if commandWidth < 1 {
		commandWidth = 1
	}
	for index := len(a.chat) - 1; index >= 0; index-- {
		message := &a.chat[index]
		if message.Role != "proposal" || message.Proposal == nil || message.CommandExpanded {
			continue
		}
		if !a.formatCommandForDisplay(message.Proposal.Command, commandWidth, 6).Truncated {
			continue
		}
		return a.expandAIConversationCommand(index)
	}
	return false
}

func (a *App) expandAIConversationCommand(chatIndex int) bool {
	if chatIndex < 0 || chatIndex >= len(a.chat) || a.chat[chatIndex].CommandExpanded {
		return false
	}
	_, _, width, _ := a.aiRegion()
	before := len(a.aiDisplayLines(width - 2))
	a.chat[chatIndex].CommandExpanded = true
	after := len(a.aiDisplayLines(width - 2))
	if added := after - before; added > 0 {
		a.aiScroll += added
	}
	a.dirty = true
	return true
}

func (a *App) handleCommandExpandClick(x, y int) bool {
	if a.compact && a.focus != focusTerminal && a.focus != focusApproval {
		return false
	}
	if a.proposal != nil {
		if a.inlineApprovalAvailable() {
			if a.inlineCommandExpandHit(a.inlineAI, x, y) {
				return a.expandInlineCommand(a.inlineAI, true)
			}
		} else {
			top, left, width, height := a.approvalGeometry()
			for index, line := range a.approvalCardInner(width, height) {
				if y == top+1+index && textHit(x, left+2, line, a.t("[展开全部]")) {
					return a.expandApprovalCardCommand()
				}
			}
		}
	}
	for _, layout := range a.buildTerminalLayout().Blocks {
		if layout.State == a.inlineAI && a.proposal != nil {
			continue
		}
		if a.inlineLayoutCommandExpandHit(layout, x, y) {
			return a.expandInlineLayoutCommand(layout, true)
		}
	}
	return false
}

func (a *App) inlineCommandExpandHit(state *inlineAIState, x, y int) bool {
	layout, ok := a.inlineLayoutFor(state)
	return ok && a.inlineLayoutCommandExpandHit(layout, x, y)
}

func (a *App) inlineLayoutCommandExpandHit(layout inlineBlockLayout, x, y int) bool {
	for index, line := range layout.Lines {
		if y == layout.Top+1+index && textHit(x, layout.Left, line, a.t("[展开全部]")) {
			return true
		}
	}
	return false
}

func (a *App) handleAICommandExpandClick(x, y int) bool {
	if a.focus == focusAIHistory {
		return false
	}
	top, left, width, height := a.aiRegion()
	if y <= top || y >= top+height {
		return false
	}
	conversationHeight := aiConversationViewport(height, a.focus == focusAI)
	row := y - (top + 1)
	if row < 0 || row >= conversationHeight {
		return false
	}
	visible := fitTailDisplayOffset(a.aiDisplayLines(width-2), conversationHeight, a.aiScroll)
	if row >= len(visible) {
		return false
	}
	line := visible[row]
	if !line.ExpandCommand || !textHit(x, left+1, line.Text, a.t("[展开全部]")) {
		return false
	}
	return a.expandAIConversationCommand(line.ChatIndex)
}
