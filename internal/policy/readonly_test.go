package policy

import "testing"

func TestReadOnlyMonitorPolicy(t *testing.T) {
	for _, command := range []string{"ps aux | head", "journalctl -n 20 --no-pager 2>/dev/null", "ss -ant"} {
		if !IsReadOnlyMonitorCommand(command) {
			t.Fatalf("expected read-only command: %s", command)
		}
	}
	for _, command := range []string{"rm -rf /tmp/a", "systemctl restart nginx", "cat /tmp/a > /etc/a"} {
		if IsReadOnlyMonitorCommand(command) {
			t.Fatalf("expected mutating command to be rejected: %s", command)
		}
	}
}
