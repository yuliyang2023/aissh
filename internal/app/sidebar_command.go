package app

import (
	"strings"
	"time"
)

func parseLocalSidebarCommand(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	const command = "/sidebar"
	if lower == command {
		return "", true
	}
	if strings.HasPrefix(lower, command+" ") || strings.HasPrefix(lower, command+"\t") {
		return strings.TrimSpace(lower[len(command):]), true
	}
	return "", false
}

func (a *App) handleLocalSidebarCommand(line string, inline bool) bool {
	argument, ok := parseLocalSidebarCommand(line)
	if !ok {
		return false
	}
	message := ""
	switch argument {
	case "", "toggle", "show", "hide":
		if a.compact {
			message = a.t("终端宽度不足 92 列，暂时无法显示右栏；加宽窗口后可用 /sidebar 切换。")
		} else {
			switch argument {
			case "", "toggle":
				a.toggleSidebar()
			case "show":
				a.ensureSidebarVisible()
			case "hide":
				if !a.sidebarCollapsed {
					a.toggleSidebar()
				}
			}
		}
	default:
		message = a.t("用法：/sidebar [toggle|show|hide]（切换、展开或收起右栏）")
	}
	if message != "" {
		if inline {
			a.showInlineLocalMessage(line, message, false)
		} else {
			a.appendPersistentChat(chatMessage{Role: "system", Content: message, Time: time.Now()})
			a.aiScroll = 0
		}
	}
	a.dirty = true
	return true
}
