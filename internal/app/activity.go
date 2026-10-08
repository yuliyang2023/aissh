package app

import (
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func (a *App) assistantStreamIndex() int {
	if !a.config.DesktopUI {
		if len(a.chat) > 0 && a.chat[len(a.chat)-1].Role == "assistant_stream" {
			return len(a.chat) - 1
		}
		return -1
	}
	for i := len(a.chat) - 1; i >= 0; i-- {
		if a.chat[i].Role == "assistant_stream" {
			return i
		}
		if a.chat[i].Role == "user" || a.chat[i].Role == "assistant" {
			break
		}
	}
	return -1
}

func (a *App) updateAgentActivity(activity agent.Activity) {
	if !a.config.DesktopUI || activity.ID == "" {
		return
	}
	for i := len(a.chat) - 1; i >= 0; i-- {
		if a.chat[i].Activity != nil && a.chat[i].Activity.ID == activity.ID {
			a.chat[i].Activity = &activity
			a.chat[i].Content = activity.Title
			return
		}
	}
	a.chat = append(a.chat, chatMessage{Role: "activity", Content: activity.Title, Activity: &activity, Time: time.Now()})
}

func (a *App) updateSSHActivity(proposalID, status, detail string) {
	if !a.config.DesktopUI {
		return
	}
	for i := len(a.chat) - 1; i >= 0; i-- {
		m := &a.chat[i]
		if m.Proposal != nil && m.Proposal.ID == proposalID {
			m.Activity = &agent.Activity{ID: "ssh:" + proposalID, Kind: "ssh", Title: m.Proposal.Command, Status: status, Detail: detail}
			return
		}
	}
}

func (a *App) finishSSHActivity(execution *executionState) {
	status, detail := "unknown", execution.completionDescription()
	if execution.ExitCode != nil {
		status = "completed"
		if *execution.ExitCode != 0 {
			status = "failed"
		}
		detail = a.t("退出码：%d", *execution.ExitCode)
	}
	if execution.Output != nil {
		detail += "\n" + execution.Output.ContextString()
	}
	a.updateSSHActivity(execution.ProposalID, status, detail)
}

func (a *App) finishAgentActivities() {
	if !a.config.DesktopUI {
		return
	}
	for i := range a.chat {
		m := &a.chat[i]
		if m.Activity != nil && m.Activity.Kind != "ssh" && m.Activity.Status == "running" {
			copy := *m.Activity
			copy.Status = "unknown"
			m.Activity = &copy
		}
	}
}

func (a *App) appendCommandDecision(message chatMessage) {
	if a.config.DesktopUI {
		a.persistChatMessage(message)
	} else {
		a.appendPersistentChat(message)
	}
}

func (a *App) sendAgentEvent(event any) {
	select {
	case a.events <- event:
	case <-a.ctx.Done():
	}
}
