package app

import (
	"context"
	"errors"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

// Notify only after the final state has been applied. In always-yes mode a
// proposal can immediately start another AI operation instead of becoming idle.
func (a *App) notifyAgentTurn(turn agent.Turn, err error) {
	if a.notifyAI == nil || a.agentBusy || a.execution != nil ||
		errors.Is(err, context.Canceled) || (a.ctx != nil && a.ctx.Err() != nil) {
		return
	}
	var title, message string
	switch {
	case err != nil || turn.Error != "" || turn.Type == "failed":
		title = a.t("AI 处理失败")
		message = a.t("请返回 aissh 查看错误信息。")
	case turn.Proposal != nil:
		title = a.t("AI 等待命令确认")
		message = a.t("命令建议已准备好，请返回 aissh 确认。")
	case turn.Type == "ask_user":
		title = a.t("AI 等待回复")
		message = a.t("AI 需要补充信息，请返回 aissh 回复。")
	case turn.Type == "approved" || turn.Type == "rejected":
		return
	default:
		title = a.t("AI 已完成")
		message = a.t("回答已准备好，请返回 aissh 查看。")
	}
	a.aiNotice = title
	a.dirty = true
	// Keep AI output and commands out of desktop notifications.
	if a.parsed.Target != "" {
		message = a.parsed.Target + "\n" + message
	}
	a.notifyAI("aissh · "+title, message)
}
