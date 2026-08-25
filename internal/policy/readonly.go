package policy

import (
	"regexp"
	"strings"
)

var mutatingPattern = regexp.MustCompile(`(?i)(?:^|[;&|]\s*)(?:sudo\s+)?(?:rm|mv|cp|install|truncate|tee|dd|mkfs|wipefs|shred|shutdown|reboot|poweroff|halt|kill|pkill|killall|mount|umount|apt|apt-get|yum|dnf|apk|rpm|dpkg|useradd|userdel|usermod|passwd|chmod|chown|systemctl\s+(?:start|stop|restart|reload|enable|disable|mask|unmask)|service\s+\S+\s+(?:start|stop|restart|reload)|docker\s+(?:run|exec|stop|restart|rm|rmi)|kubectl\s+(?:apply|create|delete|edit|exec|patch|replace|rollout|scale))\b|(?:^|[^>])>{1,2}\s*\S`)
var devNullRedirectPattern = regexp.MustCompile(`[0-9]*>{1,2}\s*/dev/null`)

// IsReadOnlyMonitorCommand is deliberately conservative. Monitor panels run
// repeatedly, so commands that may mutate state are rejected even when a user
// creates the panel manually; one-off changes belong in the SSH pane and retain
// the normal AI approval flow.
func IsReadOnlyMonitorCommand(command string) bool {
	normalized := strings.TrimSpace(command)
	normalized = devNullRedirectPattern.ReplaceAllString(normalized, "")
	if normalized == "" || mutatingPattern.MatchString(normalized) {
		return false
	}
	return true
}
