package app

import (
	"errors"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
)

type desktopCommandEvent struct {
	command credentialui.WorkbenchCommand
	reply   chan error
}

// All app state is copied on the event loop. Slow desktop IPC cannot stall SSH.
func (a *App) publishDesktopState() {
	if a.desktopStates == nil {
		return
	}
	state := a.desktopState()
	select {
	case <-a.desktopStates:
	default:
	}
	a.desktopStates <- state
}

func (a *App) desktopState() credentialui.WorkbenchState {
	state := credentialui.WorkbenchState{
		ConversationID: a.historyConversationID, Busy: a.agentBusy,
		Status: a.safeText(a.agentStatus), Executing: a.execution != nil || a.executionRecovery,
		CanApprove: a.desktopCanApprove(), ApprovalStatus: a.safeText(a.approvalStatus),
		Messages: []credentialui.WorkbenchMessage{}, Panels: []credentialui.MonitorPanel{},
	}
	if a.proposal != nil {
		p := *a.proposal
		p.Command, p.Explanation, p.ExpectedEffect = a.safeText(p.Command), a.safeText(p.Explanation), a.safeText(p.ExpectedEffect)
		state.Proposal = &credentialui.WorkbenchProposal{ID: p.ID, Command: p.Command, Explanation: p.Explanation, ExpectedEffect: p.ExpectedEffect, Risk: p.Risk}
	}
	start := max(0, len(a.chat)-100)
	for _, m := range a.chat[start:] {
		state.Messages = append(state.Messages, credentialui.WorkbenchMessage{
			Role: m.Role, Content: desktopText(a.safeText(m.Content)), Time: m.Time.UTC().Format(time.RFC3339Nano),
		})
	}
	if a.panels != nil {
		for _, p := range a.panels.Panels() {
			state.Panels = append(state.Panels, credentialui.MonitorPanel{
				ID: p.ID, Title: a.safeText(p.Title), Kind: string(p.Kind), Content: desktopText(a.safeText(p.Content)),
				Status: a.safeText(p.Status), Error: a.safeText(p.LastError), UpdatedAt: p.LastUpdated.UTC().Format(time.RFC3339Nano),
				IntervalMS: p.Interval.Milliseconds(), Pinned: p.Pinned,
			})
		}
	}
	return state
}

func desktopText(s string) string {
	runes := []rune(s)
	if len(runes) > 16000 {
		return string(runes[:16000]) + "\n…（内容较长，已截断显示）"
	}
	return s
}

func (a *App) runDesktopBridge() {
	var results []credentialui.CommandResult
	for {
		var state credentialui.WorkbenchState
		select {
		case <-a.ctx.Done():
			return
		case state = <-a.desktopStates:
		}
		commands, err := credentialui.ExchangeWorkbench(a.ctx, state, results)
		if err != nil {
			continue
		}
		results = nil
		for _, command := range commands {
			reply := make(chan error, 1)
			select {
			case <-a.ctx.Done():
				return
			case a.events <- desktopCommandEvent{command, reply}:
			}
			select {
			case <-a.ctx.Done():
				return
			case err := <-reply:
				result := credentialui.CommandResult{ID: command.ID}
				if err != nil {
					result.Error = err.Error()
				}
				results = append(results, result)
			}
		}
	}
}

func (a *App) desktopCanApprove() bool {
	return a.proposal != nil && !a.agentBusy && a.atPrompt && !a.sensitive && a.execution == nil && !a.executionRecovery &&
		a.inBandMonitor == nil && a.zmodem == nil && len(a.lineInput) == 0 && a.lineReliable && !a.terminalPasteMode
}

func (a *App) handleDesktopCommand(command credentialui.WorkbenchCommand) error {
	if !a.config.DesktopUI || a.terminalDone || command.Deadline <= time.Now().UnixMilli() {
		return errors.New("操作已过期或连接已结束")
	}
	switch command.Kind {
	case "prompt":
		prompt := strings.TrimSpace(command.Text)
		if prompt == "" || len(prompt) > 64*1024 {
			return errors.New("请输入有效的问题（最多 64 KB）")
		}
		if a.agentBusy || a.proposal != nil || a.execution != nil || a.executionRecovery {
			return errors.New("请等待当前任务完成，或先处理命令建议")
		}
		a.startAI(prompt, false)
	case "approve", "reject":
		if a.proposal == nil || a.proposal.ID != command.ProposalID || a.agentBusy {
			return errors.New("命令建议已变化，请查看最新状态")
		}
		if command.Kind == "approve" {
			if !a.desktopCanApprove() {
				return errors.New("请等待 SSH 回到空闲 Shell 提示符，并清空未提交的输入")
			}
			a.approveProposal()
		} else {
			a.rejectProposal()
		}
	case "monitor-open":
		if len(a.panels.Panels()) >= 16 {
			return errors.New("最多打开 16 个监控，请先关闭不用的监控")
		}
		var title, collector string
		interval := 3 * time.Second
		switch command.PanelID {
		case "system":
			return nil
		case "processes":
			title, collector = "进程", panel.BuiltinProcessCommand
		case "disk":
			title, collector, interval = "磁盘", panel.BuiltinDiskCommand, 10*time.Second
		case "network":
			title, collector = "网络", panel.BuiltinNetworkCommand
		case "services":
			title, collector, interval = "服务", panel.BuiltinServiceCommand, 10*time.Second
		case "custom":
			title, collector, interval = "自定义监控", strings.TrimSpace(command.Text), 5*time.Second
			if collector == "" || !policy.IsReadOnlyMonitorCommand(collector) {
				return errors.New("请输入符合只读策略的监控命令")
			}
		default:
			return errors.New("未知的监控类型")
		}
		id := "desktop-" + command.PanelID
		if command.PanelID == "custom" {
			id = ""
		}
		a.panels.Open(panel.Panel{ID: id, Kind: panel.KindMonitor, Origin: panel.OriginUser, Title: title, Command: collector, Interval: interval, Target: a.parsed.Target, AllowAIRead: true})
	case "monitor-close":
		return a.panels.Close(command.PanelID, panel.OriginUser)
	case "monitor-refresh":
		if a.panels.ByID(command.PanelID) == nil {
			return errors.New("监控不存在")
		}
		a.monitorLast[command.PanelID] = time.Time{}
	default:
		return errors.New("未知的工作台操作")
	}
	a.dirty = true
	return nil
}
