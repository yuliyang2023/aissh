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

func TestMySQLWidgetSafeIntegerComparison(t *testing.T) {
	command, err := os.ReadFile("testdata/mysql_widget_monitor.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReadOnlyMonitorCommand(string(command)); err != nil {
		t.Fatalf("MySQL widget collector rejected: %v", err)
	}
	// A valid comparison must not exempt an actual write in the same collector.
	if err := ValidateReadOnlyMonitorCommand(string(command) + " > /tmp/mysql-status.json"); err == nil {
		t.Fatal("collector output file write accepted")
	}
}

func TestAWKComparisonsDoNotBecomeFileRedirects(t *testing.T) {
	for _, command := range []string{
		`awk 'BEGIN { x=1; if(x+0>9007199254740991) print "null"; else print x }'`,
		`awk 'BEGIN { x=1; print (x > 0); printf "%d\n", (x >= 1) }'`,
		`printf 'n=3\n' | awk -F= -v max=10 '{ if($2>max) print "overflow"; else print $2 }'`,
		`/usr/bin/awk -- 'BEGIN { if (value * 100 > limit - 1) print "high" }'`,
		`sh -c 'awk "BEGIN { if(x+0>9007199254740991) print 1 }"'`,
		`printf '%s\n' "$(awk 'BEGIN { print (3>2) }')"`,
		`awk 'BEGIN { if (x>1) print 1 }' 2>/dev/null`,
	} {
		if err := ValidateReadOnlyMonitorCommand(command); err != nil {
			t.Errorf("AWK comparison %q rejected: %v", command, err)
		}
	}
}

func TestAWKComparisonExceptionRetainsWriteChecks(t *testing.T) {
	for _, command := range []string{
		`awk 'BEGIN { if(x+0>9007199254740991) print 1 }' > /tmp/output`,
		`awk 'BEGIN { if(x>0) print x > "/tmp/output" }'`,
		`awk 'BEGIN { print (x+0) > 9007199254740991 }'`,
		`awk 'BEGIN { printf "%s", (x>0) >> "/tmp/output" }'`,
		`awk 'BEGIN { if(x>0) system("echo data > /tmp/output") }'`,
		`awk 'BEGIN { if(x>0) print 1 | "echo data > /tmp/output" }'`,
		`awk 'BEGIN { if(x>0) print 1 | "(echo+0>9007199254740991)" }'`,
		`awk 'BEGIN { print 1 }' "$(echo data > /tmp/output)"`,
		`sh -c 'awk "BEGIN { if(x>0) print 1 }"; echo data > /tmp/output'`,
		`sh -c '(echo+0>9007199254740991)'`,
		`sh -c 'awk -v program="(echo+0>9007199254740991)" "BEGIN { print program }"; systemctl restart mysql'`,
		`awk 'BEGIN { if(x>0) print 1 }' 2>&1; rm -rf /tmp/data`,
		`awk "BEGIN { if(x>0) print \"$(echo data >/tmp/output)\" }"`,
		`awk 'BEGIN { if(x>0) print 1 }' > /dev/null.log`,
		`awk 'BEGIN { if(x>0) print 1 }' >/dev/null$suffix`,
		`awk 'BEGIN { if(x>0) print 1 }' 2>&10`,
	} {
		if err := ValidateReadOnlyMonitorCommand(command); err == nil {
			t.Errorf("unsafe command %q accepted", command)
		}
	}
}
