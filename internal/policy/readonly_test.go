package policy

import (
	"os"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

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

func TestMonitorOutputRedirects(t *testing.T) {
	for _, command := range []string{
		"ps aux 2>&1", "ps aux 1>&2", "ps aux >&1",
		"command -v mysql >/dev/null 2>&1; ps aux",
		"ps aux >> /dev/null", "(ps aux 2>&1)",
	} {
		if err := ValidateReadOnlyMonitorCommand(command); err != nil {
			t.Errorf("read-only command %q rejected: %v", command, err)
		}
	}
	for _, command := range []string{
		"", "ps aux > /tmp/output", "ps aux 2>&1 > /tmp/output",
		"ps aux > /dev/null.log", "ps aux >/dev/null/other", "ps aux >/dev/null$suffix",
		`ps aux >/dev/null".log"`, "ps aux 2>&10", "ps aux >2>&1",
		"ps aux 2>&1; rm -rf /tmp/data", "ps aux >/dev/null; systemctl restart mysql",
	} {
		if err := ValidateReadOnlyMonitorCommand(command); err == nil {
			t.Errorf("unsafe command %q accepted", command)
		}
	}
	err := ValidateReadOnlyMonitorCommand("ps aux > /tmp/output")
	if err == nil || !strings.Contains(err.Error(), "/tmp/output") {
		t.Fatalf("rejection does not identify offending redirect: %v", err)
	}
}

func TestBuiltinAndMySQLMonitorsPassPolicy(t *testing.T) {
	for _, command := range []string{panel.BuiltinOverviewCommand, panel.BuiltinProcessCommand,
		panel.BuiltinDiskCommand, panel.BuiltinNetworkCommand, panel.BuiltinServiceCommand} {
		if err := ValidateReadOnlyMonitorCommand(command); err != nil {
			t.Errorf("builtin monitor rejected: %v\n%s", err, command)
		}
	}
	command, err := os.ReadFile("testdata/mysql_monitor.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadOnlyMonitorCommand(string(command)); err != nil {
		t.Fatalf("original MySQL monitor rejected: %v", err)
	}
}
