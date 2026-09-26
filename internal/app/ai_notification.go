package app

import (
	"context"
	"errors"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
)

// Notify only after the final state has been applied. In always-yes mode a
// proposal can immediately start another AI operation instead of becoming idle.
func (a *App) notifyAgentTurn(turn agent.Turn, err error) {
	if (a.notifyAI == nil && !a.config.DesktopUI) || a.agentBusy || a.execution != nil || a.executionRecovery ||
		errors.Is(err, context.Canceled) || (a.ctx != nil && a.ctx.Err() != nil) {
		return
	}
	var title, message, kind string
	switch {
	case err != nil || turn.Error != "" || turn.Type == "failed":
		kind = "failed"
		title = a.t("AI 处理失败")
		message = a.t("请返回 aissh 查看错误信息。")
	case turn.Proposal != nil:
		kind = "approval"
		title = a.t("AI 等待命令确认")
		message = a.t("命令建议已准备好，请返回 aissh 确认。")
	case turn.Type == "ask_user":
		kind = "reply"
		title = a.t("AI 等待回复")
		message = a.t("AI 需要补充信息，请返回 aissh 回复。")
	case turn.Type == "approved" || turn.Type == "rejected":
		return
	default:
		kind = "completed"
		title = a.t("AI 已完成")
		message = a.t("回答已准备好，请返回 aissh 查看。")
	}
	a.aiNotice = title
	a.dirty = true
	if a.config.DesktopUI {
		a.desktopNoticeSequence++
		a.desktopNotices = append(a.desktopNotices, credentialui.WorkbenchNotice{
			Sequence: a.desktopNoticeSequence, Kind: kind, Time: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if len(a.desktopNotices) > 20 {
			a.desktopNotices = a.desktopNotices[len(a.desktopNotices)-20:]
		}
	}
	if a.notifyAI == nil {
		return
	}
	// Keep AI output and commands out of desktop notifications.
	if a.parsed.Target != "" {
		message = a.parsed.Target + "\n" + message
	}
	a.notifyAI("aissh · "+title, message)
}
