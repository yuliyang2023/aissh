package panel

import (
	"strings"
	"testing"
)

func TestAICannotCloseUserPanel(t *testing.T) {
	manager := NewManager("server")
	created := manager.Open(Panel{Kind: KindMonitor, Origin: OriginUser, Title: "用户监控"})
	if err := manager.Close(created.ID, OriginAI); err == nil {
		t.Fatal("expected AI close to be denied")
	}
}

func TestTabsCycle(t *testing.T) {
	manager := NewManager("server")
	manager.Open(Panel{ID: "process", Kind: KindTable, Origin: OriginUser, Title: "进程"})
	if manager.Active().ID != "system" {
		t.Fatalf("active = %s", manager.Active().ID)
	}
	manager.Next()
	if manager.Active().ID != "process" {
		t.Fatalf("active = %s", manager.Active().ID)
	}
	manager.Previous()
	if manager.Active().ID != "system" {
		t.Fatalf("active = %s", manager.Active().ID)
	}
}

func TestBuiltinOverviewCommandPrintsRealLineBreaks(t *testing.T) {
	if strings.Contains(BuiltinOverviewCommand, `\\n`) {
		t.Fatalf("overview command contains a literal escaped newline: %q", BuiltinOverviewCommand)
	}
	for _, want := range []string{`(%.1f%%)\n`, `(%s)\n`} {
		if !strings.Contains(BuiltinOverviewCommand, want) {
			t.Fatalf("overview command missing %q: %q", want, BuiltinOverviewCommand)
		}
	}
}
