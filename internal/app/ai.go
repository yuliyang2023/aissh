package app

import (
	"context"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	app_i18n "git.example.com/ai/wrd-ai-cli/internal/i18n"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
)

func (a *App) startInlineAI(prompt, sourceLine string, locallyCaptured bool) {
	if a.inlineAI != nil {
		a.archiveInlineAI()
	}
	anchorX, anchorY := 0, 0
	if a.lineAnchored {
		anchorX, anchorY = a.lineAnchorX, a.lineAnchorY
	} else if a.virtual != nil {
		cursor := a.virtual.Cursor()
		anchorX, anchorY = cursor.X, cursor.Y
		if !locallyCaptured {
			anchorX -= textWidth(sourceLine)
			if anchorX < 0 {
				anchorX = 0
			}
		}
	}
	a.lineAnchored = false
	a.inlineAI = &inlineAIState{
		Prompt:  strings.TrimSpace(prompt),
		AnchorX: anchorX,
		AnchorY: anchorY,
		Status:  a.t("正在连接本机 %s", a.agent.Name()),
		Started: time.Now(),
	}
	a.focus = focusTerminal
	if a.agentBusy {
		a.inlineAI.Response = a.t("AI 正在处理上一个请求，请稍后重试。")
		a.inlineAI.Done = true
		a.inlineAI.Status = ""
		a.dirty = true
		return
	}
	a.startAI(prompt, false)
}

func (a *App) startAI(prompt string, foreground bool) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	if a.agentBusy {
		a.chat = append(a.chat, chatMessage{Role: "system", Content: a.t("AI 正在处理上一个请求。"), Time: time.Now()})
		return
	}
	if foreground && a.inlineAI != nil {
		a.archiveInlineAI()
	}
	if foreground {
		a.focus = focusAI
		a.aiExpanded = true
		a.agentInline = nil
	} else {
		a.agentInline = a.inlineAI
	}
	a.appendPersistentChat(chatMessage{Role: "user", Content: prompt, Time: time.Now()})
	a.aiScroll = 0
	a.agentBusy = true
	a.agentStatus = a.t("正在连接 AI")
	a.agentThought = ""
	a.agentLanguage = app_i18n.PreferredResponseLanguage(prompt, a.localizer.Language())
	a.proposal = nil
	a.dirty = true

	event := "start"
	if a.agentSessionID != "" {
		event = "user_message"
	}
	request := agent.Request{
		Event:        event,
		SessionID:    a.agentSessionID,
		Prompt:       prompt,
		RecentOutput: agent.LimitOutputContext(a.recentOutput.ContextString(), a.config.AIContextChars),
		SystemContext: map[string]string{
			"ui_language":       a.localizer.Language(),
			"response_language": a.agentLanguage,
		},
		Target: &agent.Target{
			Type:      "linux",
			HostLabel: a.parsed.Target,
			Protocol:  "ssh",
		},
	}
	if a.agentSessionID == "" && strings.TrimSpace(a.agentSeedContext) != "" {
		request.SystemContext["conversation_history"] = a.agentSeedContext
	}
	go a.runAgentStream(request, true)
}

func (a *App) runAgentStream(request agent.Request, retrySession bool) {
	callbacks := agent.Callbacks{
		OnStatus:  func(value string) { a.events <- agentStatusEvent{status: value} },
		OnThought: func(value string) { a.events <- agentThoughtEvent{content: value} },
		OnAnswer:  func(value string) { a.events <- agentAnswerEvent{content: value} },
		OnPanelAction: func(value agent.PanelAction) {
			a.events <- agentPanelEvent{action: value}
		},
	}
	turn, err := a.agent.Stream(a.ctx, request, callbacks)
	if err != nil && retrySession && request.SessionID != "" && strings.Contains(strings.ToLower(err.Error()), "session not found") {
		request.Event = "start"
		request.SessionID = ""
		turn, err = a.agent.Stream(a.ctx, request, callbacks)
	}
	a.events <- agentTurnEvent{turn: turn, err: err}
}

func (a *App) appendAssistantDelta(content string) {
	if len(a.chat) == 0 || a.chat[len(a.chat)-1].Role != "assistant_stream" {
		a.chat = append(a.chat, chatMessage{Role: "assistant_stream", Time: time.Now()})
	}
	a.chat[len(a.chat)-1].Content += content
}

func (a *App) finishAgentTurn(turn agent.Turn, err error) {
	inline := a.agentInline
	a.agentInline = nil
	a.agentBusy = false
	a.agentStatus = ""
	a.agentThought = ""
	streamedMessage := -1
	if len(a.chat) > 0 && a.chat[len(a.chat)-1].Role == "assistant_stream" {
		streamedMessage = len(a.chat) - 1
		a.chat[streamedMessage].Role = "assistant"
	}
	if err != nil {
		if streamedMessage >= 0 {
			a.persistChatMessage(a.chat[streamedMessage])
		}
		a.appendPersistentChat(chatMessage{Role: "error", Content: err.Error(), Time: time.Now()})
		if inline != nil {
			inline.Error = err.Error()
			inline.Response = err.Error()
			inline.Done = true
			inline.Status = ""
			a.focus = focusTerminal
		} else {
			a.aiExpanded = true
		}
		a.dirty = true
		return
	}
	if turn.Session.ID != "" {
		a.agentSessionID = turn.Session.ID
		a.agentSeedContext = ""
	}
	if turn.Message != "" {
		if streamedMessage >= 0 {
			a.chat[streamedMessage].Content = turn.Message
		} else {
			a.appendPersistentChat(chatMessage{Role: "assistant", Content: turn.Message, Time: time.Now()})
		}
		if inline != nil {
			inline.Response = turn.Message
		}
	}
	if streamedMessage >= 0 {
		a.persistChatMessage(a.chat[streamedMessage])
	}
	if turn.Error != "" {
		a.appendPersistentChat(chatMessage{Role: "error", Content: turn.Error, Time: time.Now()})
		if inline != nil {
			inline.Error = turn.Error
			if strings.TrimSpace(inline.Response) == "" {
				inline.Response = turn.Error
			}
		}
	}
	a.proposal = turn.Proposal
	a.approvalCommandExpanded = false
	if turn.Proposal != nil {
		if inline != nil {
			a.reanchorInlineForReview(inline)
			proposalCopy := *turn.Proposal
			inline.Proposal = &proposalCopy
			inline.CommandExpanded = false
		}
		a.appendPersistentChat(chatMessage{
			Role:     "proposal",
			Content:  a.proposalConversationContent(*turn.Proposal),
			Time:     time.Now(),
			Proposal: turn.Proposal,
		})
		a.approvalInput = nil
		a.approvalStatus = ""
		a.terminalScroll = 0
		a.terminalRows = nil
		if !a.atPrompt || a.sensitive {
			a.approvalStatus = a.t("当前不在安全的 Shell 提示符；可输入 no 拒绝，或稍后再确认")
		}
		a.focus = focusApproval
		if isReviewRisk(turn.Proposal.Risk) {
			a.aiExpanded = true
		}
		if inline != nil {
			inline.Done = true
			inline.Status = ""
		}
	} else if turn.Type == "ask_user" || turn.Type == "failed" || turn.Error != "" {
		if inline == nil {
			a.aiExpanded = true
		}
	}
	if inline != nil && turn.Proposal == nil {
		inline.Done = true
		inline.Status = ""
		a.focus = focusTerminal
	}
	a.dirty = true
}

func (a *App) reanchorInlineForReview(state *inlineAIState) {
	if state == nil || a.virtual == nil || (state.AnchorY >= 0 && state.AnchorY < a.contentRows) {
		return
	}
	cursor := a.virtual.Cursor()
	state.AnchorX = cursor.X
	state.AnchorY = cursor.Y
}

func (a *App) approveProposal() {
	if a.proposal == nil || a.agentBusy {
		return
	}
	if !a.atPrompt || a.sensitive {
		a.approvalStatus = a.t("当前不在安全的 Shell 提示符，暂时不能执行；请输入 no 拒绝")
		return
	}
	proposal := *a.proposal
	a.agentBusy = true
	a.agentStatus = a.t("正在批准命令")
	a.approvalStatus = a.t("正在确认并准备写入 SSH……")
	go func() {
		turn, err := a.agent.Action(a.ctx, agent.Request{
			Event:      "approve",
			SessionID:  a.agentSessionID,
			ProposalID: proposal.ID,
		})
		a.events <- approvalEvent{turn: turn, command: proposal.Command, err: err}
	}()
}

func (a *App) finishApproval(event approvalEvent) {
	a.agentBusy = false
	a.agentStatus = ""
	if event.err != nil {
		a.appendPersistentChat(chatMessage{Role: "error", Content: a.t("批准失败：%s", event.err.Error()), Time: time.Now()})
		a.approvalStatus = a.t("批准失败：%s", event.err.Error())
		a.focus = focusApproval
		a.aiExpanded = true
		a.dirty = true
		return
	}
	if event.turn.Session.ID != "" {
		a.agentSessionID = event.turn.Session.ID
	}
	a.appendPersistentChat(chatMessage{Role: "user", Content: a.t("已批准执行命令：%s", event.command), Time: time.Now()})
	inlineExecution := a.inlineAI != nil
	if a.inlineAI != nil {
		a.inlineAI.ApprovalResult = a.t("已批准执行：%s", event.command)
		a.inlineAI.Done = true
		a.inlineAI.Status = ""
		a.archiveInlineAI()
	}
	proposalID := ""
	if a.proposal != nil {
		proposalID = a.proposal.ID
	}
	a.execution = &executionState{
		SessionID:  a.agentSessionID,
		ProposalID: proposalID,
		Command:    event.command,
		Output:     newContextBuffer(a.config.AIContextChars),
		StartedAt:  time.Now(),
		Inline:     inlineExecution,
	}
	a.beginCommandOutput(event.command)
	a.proposal = nil
	a.approvalInput = nil
	a.approvalStatus = ""
	a.lastCommand = event.command
	a.atPrompt = false
	a.pendingError = nil
	a.focus = focusTerminal
	_, _ = a.sshPTY.Write([]byte(isolateApprovedCommand(event.command)))
	a.dirty = true
}

// isolateApprovedCommand keeps shell-local control flow from an AI proposal
// inside a child shell. Remote side effects still apply, but exit, exec, cd,
// traps, and shell options cannot terminate or mutate the outer login shell.
func isolateApprovedCommand(command string) string {
	return isolateApprovedCommandWithLineEnding(command, approvedCommandLineEnding())
}

func isolateApprovedCommandWithLineEnding(command, lineEnding string) string {
	command = strings.ReplaceAll(command, "\r\n", "\n")
	command = strings.ReplaceAll(command, "\r", "\n")
	command = strings.ReplaceAll(command, "\n", lineEnding)
	return "(" + lineEnding + command + lineEnding + ")\r"
}

func (a *App) rejectProposal() {
	if a.proposal == nil || a.agentBusy {
		return
	}
	proposal := *a.proposal
	a.agentBusy = true
	a.agentStatus = a.t("正在拒绝命令")
	a.approvalStatus = a.t("正在拒绝命令建议……")
	go func() {
		turn, err := a.agent.Action(a.ctx, agent.Request{
			Event:      "reject",
			SessionID:  a.agentSessionID,
			ProposalID: proposal.ID,
		})
		a.events <- rejectionEvent{turn: turn, command: proposal.Command, err: err}
	}()
}

func (a *App) finishRejection(event rejectionEvent) {
	if a.inlineAI != nil && event.err == nil {
		a.inlineAI.ApprovalResult = a.t("已拒绝命令建议：%s", event.command)
		a.inlineAI.Done = true
		a.inlineAI.Status = ""
	}
	a.proposal = nil
	a.approvalInput = nil
	a.approvalStatus = ""
	a.agentBusy = false
	a.agentStatus = ""
	a.focus = focusTerminal
	if event.err != nil {
		if a.inlineAI != nil {
			a.inlineAI.ApprovalResult = a.t("拒绝失败：%s", event.err.Error())
			a.inlineAI.Done = true
			a.inlineAI.Status = ""
		}
		a.appendPersistentChat(chatMessage{Role: "error", Content: a.t("拒绝失败：%s", event.err.Error()), Time: time.Now()})
	} else {
		a.appendPersistentChat(chatMessage{Role: "user", Content: a.t("已拒绝命令建议：%s", event.command), Time: time.Now()})
		a.finishAgentTurn(event.turn, nil)
	}
	a.dirty = true
}

func (a *App) archiveInlineAI() {
	if a.inlineAI == nil {
		return
	}
	a.inlineHistory = append(a.inlineHistory, a.inlineAI)
	if len(a.inlineHistory) > 32 {
		a.inlineHistory = append([]*inlineAIState(nil), a.inlineHistory[len(a.inlineHistory)-32:]...)
	}
	a.inlineAI = nil
}

func (a *App) proposalConversationContent(proposal agent.Proposal) string {
	lines := []string{firstText(proposal.Explanation, proposal.ExpectedEffect, a.t("已生成命令建议，请在左侧 SSH 窗口确认。"))}
	if strings.TrimSpace(proposal.Command) != "" {
		lines = append(lines, a.t("命令：%s", proposal.Command))
	}
	if strings.TrimSpace(proposal.Risk) != "" {
		lines = append(lines, a.t("风险：%s", proposal.Risk))
	}
	return strings.Join(lines, "\n")
}

func (a *App) submitExecutionResult() {
	execution := a.execution
	if execution == nil || a.agentBusy {
		return
	}
	a.execution = nil
	if execution.Inline {
		a.startInlineExecutionAnalysis(execution.Command)
	}
	a.agentBusy = true
	a.agentStatus = a.t("AI 正在分析执行结果")
	output := ""
	if execution.Output != nil {
		output = agent.LimitOutputContext(execution.Output.ContextString(), a.config.AIContextChars)
	}
	request := agent.Request{
		Event:      "result",
		SessionID:  execution.SessionID,
		ProposalID: execution.ProposalID,
		Output:     output,
		Completion: "prompt",
		SystemContext: map[string]string{
			"ui_language":       a.localizer.Language(),
			"response_language": firstText(a.agentLanguage, a.localizer.Language()),
		},
	}
	go func() {
		callbacks := agent.Callbacks{
			OnStatus:  func(value string) { a.events <- agentStatusEvent{status: value} },
			OnThought: func(value string) { a.events <- agentThoughtEvent{content: value} },
			OnAnswer:  func(value string) { a.events <- agentAnswerEvent{content: value} },
			OnPanelAction: func(value agent.PanelAction) {
				a.events <- agentPanelEvent{action: value}
			},
		}
		turn, err := a.agent.Stream(context.Background(), request, callbacks)
		a.events <- executionResultEvent{turn: turn, err: err}
	}()
}

func (a *App) startInlineExecutionAnalysis(command string) {
	if a.inlineAI != nil {
		a.archiveInlineAI()
	}
	anchorX, anchorY := 0, 0
	if a.virtual != nil {
		cursor := a.virtual.Cursor()
		anchorX, anchorY = cursor.X, cursor.Y
	}
	a.inlineAI = &inlineAIState{
		Prompt:       a.t("AI 复核命令：%s", strings.TrimSpace(command)),
		AnchorX:      anchorX,
		AnchorY:      anchorY,
		Status:       a.t("正在分析命令执行结果"),
		StatusPrefix: a.t("分析命令结果"),
		Started:      time.Now(),
	}
	a.agentInline = a.inlineAI
	a.focus = focusTerminal
}

func (a *App) finishExecutionResult(event executionResultEvent) {
	a.finishAgentTurn(event.turn, event.err)
}

func (a *App) applyPanelAction(action agent.PanelAction) {
	spec := action.Panel
	switch strings.ToLower(action.Action) {
	case "open", "create":
		kind := panelKind(spec.Type)
		if kind == panel.KindReport {
			return
		}
		command := collectorCommand(spec.Collector)
		if command == "" {
			command = strings.TrimSpace(spec.Command)
		}
		if command != "" && !policy.IsReadOnlyMonitorCommand(command) {
			a.appendPersistentChat(chatMessage{Role: "error", Content: a.t("AI 请求的监控命令被只读策略拒绝。"), Time: time.Now()})
			return
		}
		interval := time.Duration(spec.IntervalMS) * time.Millisecond
		if interval < time.Second {
			interval = 2 * time.Second
		}
		a.panels.Open(panel.Panel{
			ID:          spec.ID,
			Kind:        kind,
			Origin:      panel.OriginAI,
			Title:       firstText(spec.Title, a.t("AI 面板")),
			Command:     command,
			Interval:    interval,
			Content:     spec.Content,
			AllowAIRead: spec.AllowAIRead,
			Target:      a.parsed.Target,
			Unread:      true,
		})
	case "update", "append":
		item := a.panels.ByID(spec.ID)
		if item == nil || item.Kind == panel.KindReport {
			return
		}
		content := spec.Content
		if strings.ToLower(action.Action) == "append" {
			content = strings.TrimSpace(item.Content + "\n\n" + spec.Content)
		}
		a.panels.UpdateContent(item.ID, content, a.t("AI 更新"), false)
	case "close":
		_ = a.panels.Close(spec.ID, panel.OriginAI)
	}
	a.dirty = true
}

func panelKind(value string) panel.Kind {
	switch strings.ToLower(value) {
	case "log":
		return panel.KindLog
	case "table":
		return panel.KindTable
	case "report":
		return panel.KindReport
	case "shell":
		return panel.KindShell
	default:
		return panel.KindMonitor
	}
}

func collectorCommand(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "system", "system.overview", "system.cpu", "system.memory", "system.load":
		return panel.BuiltinOverviewCommand
	case "process", "process.top":
		return panel.BuiltinProcessCommand
	case "disk", "disk.usage", "disk.io":
		return panel.BuiltinDiskCommand
	case "network", "network.traffic", "network.connections":
		return panel.BuiltinNetworkCommand
	case "service", "service.status":
		return panel.BuiltinServiceCommand
	case "nginx.errors":
		return `tail -n 80 /var/log/nginx/error.log 2>/dev/null`
	default:
		return ""
	}
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isReviewRisk(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "critical", "destructive", "dangerous":
		return true
	default:
		return false
	}
}
