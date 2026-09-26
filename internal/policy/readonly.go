package policy

import (
	"fmt"
	"regexp"
	"strings"
)

var mutatingPattern = regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:rm|mv|cp|install|truncate|tee|dd|mkfs|wipefs|shred|shutdown|reboot|poweroff|halt|kill|pkill|killall|mount|umount|apt|apt-get|yum|dnf|apk|rpm|dpkg|useradd|userdel|usermod|passwd|chmod|chown|systemctl\s+(?:start|stop|restart|reload|enable|disable|mask|unmask)|service\s+\S+\s+(?:start|stop|restart|reload)|docker\s+(?:run|exec|stop|restart|rm|rmi)|kubectl\s+(?:apply|create|delete|edit|exec|patch|replace|rollout|scale))\b|(?:^|[^>])>{1,2}\s*\S+`)
var harmlessRedirectPattern = regexp.MustCompile(`(?:[0-9]*>{1,2}[ \t]*/dev/null|[12]?>&[12])`)

// IsReadOnlyMonitorCommand is deliberately conservative. Monitor panels run
// repeatedly, so commands that may mutate state are rejected even when a user
// creates the panel manually; one-off changes belong in the SSH pane and retain
// the normal AI approval flow.
func IsReadOnlyMonitorCommand(command string) bool {
	return ValidateReadOnlyMonitorCommand(command) == nil
}

// ValidateReadOnlyMonitorCommand also explains rejections to the UI and agent.
// This remains a conservative textual policy, not a full shell interpreter.
func ValidateReadOnlyMonitorCommand(command string) error {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return fmt.Errorf("监控命令不能为空")
	}
	normalized = maskMonitorAWKComparisons(normalized, 0)
	normalized = stripHarmlessRedirects(normalized)
	if match := mutatingPattern.FindString(normalized); match != "" {
		return fmt.Errorf("检测到可能修改状态的命令或文件重定向：%q", strings.TrimSpace(match))
	}
	return nil
}

func stripHarmlessRedirects(command string) string {
	var result strings.Builder
	last := 0
	for _, location := range harmlessRedirectPattern.FindAllStringIndex(command, -1) {
		start, end := location[0], location[1]
		// Require whole literal tokens. Never exempt /dev/null.log, /dev/null$x,
		// 2>&10, or a descriptor-looking string that is another redirect's target.
		if start > 0 && !strings.ContainsRune(" \t\r\n;|&(", rune(command[start-1])) {
			continue
		}
		if end < len(command) && !strings.ContainsRune(" \t\r\n;|&)", rune(command[end])) {
			continue
		}
		result.WriteString(command[last:start])
		result.WriteByte(' ')
		last = end
	}
	result.WriteString(command[last:])
	return result.String()
}
