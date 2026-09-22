package app

import (
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

const (
	aiCanvasStyle        = "\x1b[38;5;250m\x1b[48;5;234m"
	aiHeaderStyle        = "\x1b[1;38;5;255m\x1b[48;5;53m"
	aiAssistantTopStyle  = "\x1b[1;38;5;213m\x1b[48;5;235m"
	aiAssistantStyle     = "\x1b[38;5;253m\x1b[48;5;235m"
	aiUserTopStyle       = "\x1b[1;38;5;117m\x1b[48;5;17m"
	aiUserStyle          = "\x1b[38;5;255m\x1b[48;5;17m"
	aiSystemTopStyle     = "\x1b[1;38;5;150m\x1b[48;5;236m"
	aiSystemStyle        = "\x1b[38;5;249m\x1b[48;5;236m"
	aiErrorTopStyle      = "\x1b[1;38;5;210m\x1b[48;5;52m"
	aiErrorStyle         = "\x1b[38;5;255m\x1b[48;5;52m"
	aiReviewTopStyle     = "\x1b[1;38;5;220m\x1b[48;5;235m"
	aiReviewStyle        = "\x1b[38;5;252m\x1b[48;5;235m"
	aiDangerReviewStyle  = "\x1b[38;5;255m\x1b[48;5;52m"
	aiDangerCodeStyle    = "\x1b[1;38;5;210m\x1b[48;5;233m"
	aiCodeStyle          = "\x1b[38;5;222m\x1b[48;5;233m"
	aiMutedStyle         = "\x1b[38;5;245m\x1b[48;5;234m"
	aiInputTopStyle      = "\x1b[1;38;5;81m\x1b[48;5;236m"
	aiInputStyle         = "\x1b[38;5;255m\x1b[48;5;236m"
	aiHistoryStyle       = "\x1b[38;5;252m\x1b[48;5;234m"
	aiHistoryActiveStyle = "\x1b[1;38;5;231m\x1b[48;5;24m"
)

type aiDisplayLine struct {
	Text          string
	Style         string
	ExpandCommand bool
	ChatIndex     int
}

func (a *App) aiHeaderLine(width int) string {
	left := a.t("✦ AI 助手")
	status := a.t("○ 待命")
	if a.focus == focusAIHistory {
		status = a.t("会话历史")
	} else if a.agentStatus != "" {
		status = "● " + a.agentStatus
	} else if a.agentBusy {
		status = a.t("● 正在处理")
	} else if a.hasAIConversation() {
		status = a.t("● 已同步")
	}
	button := a.aiHistoryButton()
	buttonWidth := textWidth(button)
	if width <= buttonWidth {
		return terminalview.ClipPad(button, width)
	}
	prefixWidth := width - buttonWidth - 1
	prefix := left + "  " + status
	if textWidth(prefix) > prefixWidth {
		prefix = terminalview.ClipPad(prefix, prefixWidth)
	} else {
		prefix += strings.Repeat(" ", prefixWidth-textWidth(prefix))
	}
	return prefix + " " + button
}

func (a *App) aiHistoryButton() string { return a.t("[会话]") }

func (a *App) aiHistoryHeaderStart() int {
	_, left, width, _ := a.aiRegion()
	return left + width - textWidth(a.aiHistoryButton())
}

func (a *App) moveHistorySelection(delta int) {
	maximum := len(a.historyConversations)
	a.historySelection += delta
	if a.historySelection < 0 {
		a.historySelection = maximum
	}
	if a.historySelection > maximum {
		a.historySelection = 0
	}
}

func (a *App) historyPickerWindow(available int) (int, int) {
	total := len(a.historyConversations) + 1
	if available <= 0 || total == 0 {
		return 0, 0
	}
	if available > total {
		available = total
	}
	selection := a.historySelection
	if selection < 0 {
		selection = 0
	}
	if selection >= total {
		selection = total - 1
	}
	start := selection - available/2
	if start < 0 {
		start = 0
	}
	if start+available > total {
		start = total - available
	}
	return start, start + available
}

func (a *App) aiHistoryRows(width, available int) []aiDisplayLine {
	start, end := a.historyPickerWindow(available)
	rows := make([]aiDisplayLine, 0, end-start)
	for index := start; index < end; index++ {
		style := aiHistoryStyle
		marker := "  "
		if index == a.historySelection {
			style = aiHistoryActiveStyle
			marker = "› "
		}
		label := a.t("+ 新建会话")
		if index > 0 {
			conversation := a.historyConversations[index-1]
			updated := conversation.UpdatedAt.Local().Format("01-02 15:04")
			if conversation.UpdatedAt.IsZero() {
				updated = a.t("未知时间")
			}
			current := ""
			if conversation.ID == a.historyConversationID {
				current = a.t(" · 当前")
			}
			label = a.t("%s  %s · %d 条%s", updated, conversation.Title, len(conversation.Messages), current)
		}
		rows = append(rows, aiDisplayLine{Text: terminalview.ClipPad(marker+label, width), Style: style})
	}
	return rows
}

func (a *App) aiDisplayLines(width int) []aiDisplayLine {
	if width <= 0 {
		return nil
	}
	return a.cachedAIConversationLines(width)
}

func (a *App) buildAIConversationLines(width int) []aiDisplayLine {
	var lines []aiDisplayLine
	for chatIndex := range a.chat {
		message := a.chat[chatIndex]
		switch {
		case message.Role == "proposal" && message.Proposal != nil:
			lines = a.appendProposalCard(lines, message, width, chatIndex)
		case message.Role == "user":
			lines = appendMessageCard(lines, a.t("你"), message.Content, message.Time, message.Historical, width, aiUserTopStyle, aiUserStyle)
		case message.Role == "assistant" || message.Role == "assistant_stream":
			if strings.TrimSpace(message.Content) != "" {
				lines = appendMessageCard(lines, "✦ AI", message.Content, message.Time, message.Historical, width, aiAssistantTopStyle, aiAssistantStyle)
			}
		case message.Role == "error":
			lines = appendMessageCard(lines, a.t("! 错误"), message.Content, message.Time, message.Historical, width, aiErrorTopStyle, aiErrorStyle)
		default:
			lines = appendMessageCard(lines, a.t("◆ 系统"), message.Content, message.Time, message.Historical, width, aiSystemTopStyle, aiSystemStyle)
		}
	}
	if strings.TrimSpace(a.agentThought) != "" {
		thought := terminalview.Wrap("◌ "+cleanMarkdownInline(a.agentThought), width-2)
		if len(thought) > 3 {
			thought = thought[len(thought)-3:]
		}
		for _, line := range thought {
			lines = append(lines, aiDisplayLine{Text: "  " + line, Style: aiMutedStyle})
		}
	}
	if a.proposal != nil && isReviewRisk(a.proposal.Risk) {
		lines = append(lines,
			aiDisplayLine{Text: a.t("  ! 高风险命令正在左侧等待确认"), Style: aiErrorTopStyle},
			aiDisplayLine{Text: "", Style: aiCanvasStyle},
		)
	}
	return lines
}

func (a *App) aiLines(width int) []string {
	display := a.aiDisplayLines(width)
	lines := make([]string, 0, len(display))
	for _, line := range display {
		lines = append(lines, line.Text)
	}
	return lines
}

func appendMessageCard(lines []aiDisplayLine, label, content string, timestamp time.Time, historical bool, width int, topStyle, bodyStyle string) []aiDisplayLine {
	if strings.TrimSpace(content) == "" {
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1].Text != "" {
		lines = append(lines, aiDisplayLine{Text: "", Style: aiCanvasStyle})
	}
	title := "╭─ " + label
	if !timestamp.IsZero() {
		format := "15:04"
		if historical {
			format = "2006-01-02 15:04"
		}
		title += " · " + timestamp.Format(format)
	}
	title += " "
	lines = append(lines, aiDisplayLine{Text: fillChatRule(title, width), Style: topStyle})
	lines = appendMarkdownBody(lines, content, width, topStyle, bodyStyle)
	lines = append(lines,
		aiDisplayLine{Text: fillChatRule("╰", width), Style: topStyle},
		aiDisplayLine{Text: "", Style: aiCanvasStyle},
	)
	return lines
}

func (a *App) appendProposalCard(lines []aiDisplayLine, message chatMessage, width, chatIndex int) []aiDisplayLine {
	proposal := message.Proposal
	if proposal == nil {
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1].Text != "" {
		lines = append(lines, aiDisplayLine{Text: "", Style: aiCanvasStyle})
	}
	topStyle, bodyStyle, codeStyle := aiReviewTopStyle, aiReviewStyle, aiCodeStyle
	if isReviewRisk(proposal.Risk) {
		topStyle, bodyStyle, codeStyle = aiErrorTopStyle, aiDangerReviewStyle, aiDangerCodeStyle
	}
	title := a.t("╭─ ⚑ 命令审查 · %s ", a.riskDisplay(proposal.Risk))
	lines = append(lines, aiDisplayLine{Text: fillChatRule(title, width), Style: topStyle})
	explanation := firstText(proposal.Explanation, proposal.ExpectedEffect, a.t("AI 已生成命令建议"))
	lines = appendWrappedCardLine(lines, "│ ", cleanMarkdownInline(explanation), width, bodyStyle)
	lines = append(lines, aiDisplayLine{Text: "│", Style: bodyStyle})
	maximumLines := 6
	if message.CommandExpanded {
		maximumLines = 0
	}
	command := a.formatCommandForDisplay(proposal.Command, maxInt(width-4, 1), maximumLines)
	for index, line := range command.Lines {
		displayLine := aiDisplayLine{Text: "│  " + line, Style: codeStyle}
		if command.Truncated && index == len(command.Lines)-1 {
			displayLine.ExpandCommand = true
			displayLine.ChatIndex = chatIndex
		}
		lines = append(lines, displayLine)
	}
	if proposal.ExpectedEffect != "" && proposal.ExpectedEffect != explanation {
		lines = appendWrappedCardLine(lines, a.t("│ 预期  "), cleanMarkdownInline(proposal.ExpectedEffect), width, bodyStyle)
	}
	lines = append(lines,
		aiDisplayLine{Text: fillChatRule("╰", width), Style: topStyle},
		aiDisplayLine{Text: "", Style: aiCanvasStyle},
	)
	return lines
}

func appendMarkdownBody(lines []aiDisplayLine, content string, width int, headingStyle, bodyStyle string) []aiDisplayLine {
	inCode := false
	for _, source := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		trimmed := strings.TrimSpace(source)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			lines = appendWrappedCardLine(lines, "│  ", source, width, aiCodeStyle)
			continue
		}
		if trimmed == "" {
			lines = append(lines, aiDisplayLine{Text: "│", Style: bodyStyle})
			continue
		}
		if heading, ok := markdownHeading(trimmed); ok {
			lines = appendWrappedCardLine(lines, "│ ◆ ", heading, width, headingStyle)
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			lines = appendWrappedCardLine(lines, "│ • ", cleanMarkdownInline(strings.TrimSpace(trimmed[2:])), width, bodyStyle)
			continue
		}
		if strings.HasPrefix(trimmed, "> ") {
			lines = appendWrappedCardLine(lines, "│ ▍ ", cleanMarkdownInline(strings.TrimSpace(trimmed[2:])), width, aiMutedStyle)
			continue
		}
		lines = appendWrappedCardLine(lines, "│ ", cleanMarkdownInline(source), width, bodyStyle)
	}
	return lines
}

func appendWrappedCardLine(lines []aiDisplayLine, prefix, content string, width int, style string) []aiDisplayLine {
	continuation := strings.Repeat(" ", textWidth(prefix))
	if strings.HasPrefix(prefix, "│") {
		continuation = "│" + strings.Repeat(" ", maxInt(textWidth(prefix)-1, 0))
	}
	available := width - textWidth(prefix)
	if available < 1 {
		available = 1
	}
	wrapped := terminalview.Wrap(content, available)
	if len(wrapped) == 0 {
		wrapped = []string{""}
	}
	for index, line := range wrapped {
		linePrefix := prefix
		if index > 0 {
			linePrefix = continuation
		}
		lines = append(lines, aiDisplayLine{Text: linePrefix + line, Style: style})
	}
	return lines
}

func markdownHeading(value string) (string, bool) {
	trimmed := strings.TrimLeft(value, "#")
	if trimmed == value || trimmed == "" || trimmed[0] != ' ' {
		return "", false
	}
	return cleanMarkdownInline(strings.TrimSpace(trimmed)), true
}

func cleanMarkdownInline(value string) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{"**", "__", "`"} {
		value = strings.ReplaceAll(value, marker, "")
	}
	return value
}

func fillChatRule(prefix string, width int) string {
	remaining := width - textWidth(prefix)
	if remaining <= 0 {
		return terminalview.ClipPad(prefix, width)
	}
	return prefix + strings.Repeat("─", remaining)
}

func aiConversationViewport(height int, focused bool) int {
	_ = focused
	viewport := height - 2 - aiInputBodyRows(height)
	if viewport < 0 {
		return 0
	}
	return viewport
}

func fitTailDisplayOffset(lines []aiDisplayLine, height, offset int) []aiDisplayLine {
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

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (a *App) aiInputTitle(busy bool, width int) string {
	label := a.t("╭─ 输入")
	if busy {
		label += a.t(" · AI 正在处理")
	}
	label += " "
	return fillChatRule(label, width)
}

func (a *App) compactAIHint(expanded bool) string {
	if expanded {
		return a.t("› AI 对话已打开")
	}
	return a.t("› AI 完整对话")
}
