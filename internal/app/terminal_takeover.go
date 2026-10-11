package app

import (
	"context"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

type agentOperation struct {
	ctx        context.Context
	generation uint64
	events     chan interface{}
}
type agentOperationEvent struct {
	generation uint64
	event      interface{}
}

func (a *App) beginAgentOperation() agentOperation {
	if a.agentCancel != nil {
		a.agentCancel()
	}
	a.agentGeneration++
	ctx, cancel := context.WithCancel(a.ctx)
	a.agentCancel = cancel
	return agentOperation{ctx: ctx, generation: a.agentGeneration, events: a.events}
}

func (op agentOperation) send(event interface{}) {
	if op.ctx.Err() != nil {
		return
	}
	select {
	case op.events <- agentOperationEvent{generation: op.generation, event: event}:
	case <-op.ctx.Done():
	}
}

// Cancel local automation without sending a control character to the gateway.
// The user decides whether to enter Q, finish approval, or interrupt remotely.
func (a *App) takeOverTerminal(reason string) {
	if a.agentCancel != nil {
		a.agentCancel()
		a.agentCancel = nil
	}
	a.agentGeneration++
	a.finishAgentActivities()
	a.agentBusy = false
	a.agentStatus, a.agentThought = "", ""
	a.agentInline = nil
	if a.inlineAI != nil {
		a.inlineAI.Done = true
		a.inlineAI.Status = ""
		a.archiveInlineAI()
	}
	if a.proposal != nil {
		a.updateSSHActivity(a.proposal.ID, "cancelled", reason)
	}
	a.proposal = nil
	a.approvalInput, a.approvalStatus = nil, ""
	a.invalidatePipelines()
	if state := a.execution; state != nil {
		if state.WriteJob != nil {
			state.WriteJob.cancelled.Store(true)
		}
		a.updateSSHActivity(state.ProposalID, "unknown", "已交回终端；远端命令的退出状态未知")
		a.retireExecution(state)
		a.execution = nil
	}
	a.executionRecovery, a.executionRecoveryOutput = false, false
	a.executionInterrupt = nil
	if a.inBandMonitor != nil {
		if a.inBandMonitor.writeJob != nil {
			a.inBandMonitor.writeJob.cancelled.Store(true)
		}
		a.pauseInBandMonitor(context.Canceled)
	}
	a.monitorPrimaryPaused = true
	a.monitorAwaitingPrompt = false
	a.monitorDeferredInput, a.monitorPrompt = nil, nil
	a.lineInput, a.terminalKeyCarry = nil, nil
	a.lineCursor = 0
	a.lineAnchored, a.aiLineCapture = false, false
	a.lineReliable = true
	a.focus = focusTerminal
	a.atPrompt, a.gatewayShellPrompt = false, false
	a.terminalTakeover = true
	a.pendingError, a.errorCarry = nil, ""
	a.outputTail.value = ""
	a.appendPersistentChat(chatMessage{Role: "system", Content: reason + "；已暂停 AI 和自动采集，可直接在终端输入。远端命令不会因此自动结束。", Time: time.Now()})
	a.dirty = true
}

func (a *App) observeGatewayApproval(data []byte) bool {
	if a.zmodem != nil {
		return false
	}
	a.interactionTail += string(data)
	if len(a.interactionTail) > 8192 {
		a.interactionTail = a.interactionTail[len(a.interactionTail)-8192:]
	}
	plain := trigger.StripANSI(a.interactionTail)
	line := plain[strings.LastIndexAny(plain, "\r\n")+1:]
	if a.terminalTakeover || trigger.LooksLikeShellPrompt(line) {
		return false
	}
	if !strings.Contains(plain, "金库式审批") && !strings.Contains(plain, "选择金库授权模式") && !strings.Contains(strings.ReplaceAll(plain, " ", ""), "输入Q退出本次申请") {
		return false
	}
	hidden := a.inBandMonitor != nil && !a.inBandMonitor.visible
	a.takeOverTerminal("检测到堡垒机金库审批，需要人工操作")
	a.gatewayApproval = true
	if hidden {
		// A collector may already have swallowed fragmented approval output.
		start := strings.Index(a.interactionTail, "你所操作")
		if start < 0 {
			start = strings.Index(a.interactionTail, "选择金库授权模式")
		}
		if start < 0 {
			start = 0
		}
		a.processVisibleSSHOutput([]byte(a.interactionTail[start:]))
		return true
	}
	return false
}
