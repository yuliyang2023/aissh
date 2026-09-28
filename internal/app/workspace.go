package app

import (
	"fmt"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/history"
)

// localWorkspaceAgent is implemented by AI providers that can read a local
// code workspace (currently CodexProvider and ClaudeProvider).
type localWorkspaceAgent interface {
	Name() string
	SetLocalWorkspace(string)
	LocalWorkspace() string
}

func parseLocalWorkspaceCommand(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, command := range []string{"/workspace", "/ws"} {
		if lower == command {
			return "", true
		}
		if strings.HasPrefix(lower, command+" ") || strings.HasPrefix(lower, command+"\t") {
			return strings.TrimSpace(trimmed[len(command):]), true
		}
	}
	return "", false
}

func (a *App) handleLocalWorkspaceCommand(line string, inline bool) bool {
	argument, ok := parseLocalWorkspaceCommand(line)
	if !ok {
		return false
	}
	message, err := a.applyLocalWorkspaceCommand(argument)
	role := "system"
	if err != nil {
		role = "error"
		message = err.Error()
	}
	a.appendPersistentChat(chatMessage{Role: role, Content: message, Time: time.Now()})
	if inline {
		a.showInlineLocalMessage(line, message, err != nil)
	}
	a.aiScroll = 0
	a.dirty = true
	return true
}

func (a *App) applyLocalWorkspaceCommand(argument string) (string, error) {
	provider, ok := a.agent.(localWorkspaceAgent)
	if !ok {
		return "", fmt.Errorf("%s", a.t("当前 AI 后端不支持本机代码工作区"))
	}
	argument = trimMatchingQuotes(strings.TrimSpace(argument))
	if argument == "" {
		if workspace := provider.LocalWorkspace(); workspace != "" {
			if a.config.WorkspaceWritable {
				return a.t("当前本机代码工作区（可修改）：%s", workspace), nil
			}
			return a.t("当前本机代码工作区（只读）：%s", workspace), nil
		}
		return a.t("当前未启用本机代码工作区；使用 /workspace 本机路径 开启"), nil
	}
	if a.agentBusy || a.proposal != nil || a.execution != nil || a.executionRecovery {
		return "", fmt.Errorf("%s", a.t("AI 正在处理请求；请在本次请求结束后切换工作区"))
	}
	if workspaceDisabledArgument(argument) {
		a.setLocalWorkspace(provider, "")
		return a.t("已关闭本机代码工作区；后续 %s 请求不会读取本机项目", provider.Name()), nil
	}
	workspace, err := resolveLocalWorkspace(argument)
	if err != nil {
		return "", fmt.Errorf("%s", a.t("无法访问本机代码工作区：%s", err.Error()))
	}
	a.setLocalWorkspace(provider, workspace)
	if a.config.WorkspaceWritable {
		return a.t("本机代码工作区已切换（可修改）：%s；后续 %s 请求将使用此目录", workspace, provider.Name()), nil
	}
	return a.t("本机代码工作区已切换（只读）：%s；后续 %s 请求将使用此目录", workspace, provider.Name()), nil
}

func (a *App) setLocalWorkspace(provider localWorkspaceAgent, workspace string) {
	if provider.LocalWorkspace() != workspace {
		// A directory change starts a fresh provider session with the visible chat
		// as context, so the old provider session cannot retain its previous cwd.
		messages := make([]history.Message, 0, len(a.chat))
		for _, message := range a.chat {
			messages = append(messages, history.Message{Role: message.Role, Content: message.Content})
		}
		a.agentSessionID = ""
		a.agentSeedContext = historyConversationContext(messages)
	}
	provider.SetLocalWorkspace(workspace)
	a.config.Workspace = workspace
}

func workspaceDisabledArgument(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "none", "disable", "disabled", "关闭", "禁用":
		return true
	default:
		return false
	}
}

func trimMatchingQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
		return value[1 : len(value)-1]
	}
	return value
}

func (a *App) showInlineLocalMessage(line, message string, failed bool) {
	if a.inlineAI != nil {
		a.archiveInlineAI()
	}
	anchorX, anchorY := 0, 0
	if a.lineAnchored {
		anchorX, anchorY = a.lineAnchorX, a.lineAnchorY
	} else if a.virtual != nil {
		cursor := a.virtual.Cursor()
		anchorX, anchorY = cursor.X, cursor.Y
		anchorX -= textWidth(line)
		if anchorX < 0 {
			anchorX = 0
		}
	}
	a.lineAnchored = false
	state := &inlineAIState{
		Prompt:   strings.TrimSpace(line),
		AnchorX:  anchorX,
		AnchorY:  anchorY,
		Response: message,
		Done:     true,
		Started:  time.Now(),
	}
	if failed {
		state.Error = message
	}
	a.inlineAI = state
	a.focus = focusTerminal
}
