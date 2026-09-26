package alerts

import (
	"fmt"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

var epoch = time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

func memoryEngine() *Engine {
	p := DefaultPolicy()
	p.Rules = []Rule{{ID: "memory", Name: "内存使用率", Metric: "memory", Enabled: true, Threshold: 90, Severity: "warning"}}
	p.RepeatMinutes = 1
	return New(p)
}
func sample(at time.Time, memory float64) Session {
	return Session{ID: "s1", Name: "生产", Status: "running", CreatedAt: epoch, Panels: []credentialui.MonitorPanel{{ID: "system", Title: "系统", IntervalMS: 2000, UpdatedAt: at.Format(time.RFC3339Nano), Content: fmt.Sprintf("CORES=4\nLOAD=8 4 2\nMEM=1/2 MB (%.1f%%)\nDISK=1/2 (20%%)", memory)}}}
}
func tick(e *Engine, seconds int, memory float64) {
	now := epoch.Add(time.Duration(seconds) * time.Second)
	e.Tick(now, []Session{sample(now, memory)})
}
func TestSustainedThresholdFreshSamplesHysteresisAndRecovery(t *testing.T) {
	e := memoryEngine()
	tick(e, 0, 95)
	e.Tick(epoch.Add(10*time.Second), []Session{sample(epoch, 95)})
	if len(e.Snapshot().Incidents) != 0 {
		t.Fatal("a duplicate snapshot triggered an incident")
	}
	tick(e, 11, 95)
	if len(e.Snapshot().Notices) != 1 {
		t.Fatal("sustained fresh samples did not notify")
	}
	tick(e, 20, 89)
	tick(e, 31, 89)
	if e.Snapshot().Incidents[0].Status != "firing" {
		t.Fatal("hysteresis band recovered")
	}
	tick(e, 32, 80)
	e.Tick(epoch.Add(43*time.Second), []Session{sample(epoch.Add(32*time.Second), 80)})
	if e.Snapshot().Incidents[0].Status != "firing" {
		t.Fatal("duplicate healthy sample recovered")
	}
	tick(e, 44, 80)
	state := e.Snapshot()
	if len(state.Notices) != 2 || state.Notices[1].Kind != "recovered" || state.Incidents[0].Status != "recovered" {
		t.Fatalf("unexpected recovery: %+v", state)
	}
	tick(e, 50, 95)
	tick(e, 61, 95)
	if len(e.Snapshot().Notices) != 3 || len(e.Snapshot().Incidents) != 2 {
		t.Fatal("a recurrence did not create a new incident")
	}
}
func TestSpikesUnknownStaleAndFailedCollectionCannotRecover(t *testing.T) {
	e := memoryEngine()
	tick(e, 0, 95)
	tick(e, 5, 20)
	tick(e, 11, 95)
	if len(e.Snapshot().Incidents) != 0 {
		t.Fatal("brief spike triggered")
	}
	tick(e, 22, 95)
	for _, mode := range []string{"missing", "error", "stale", "malformed", "offline"} {
		s := sample(epoch.Add(30*time.Second), 1)
		switch mode {
		case "missing":
			s.Panels = nil
		case "error":
			s.Panels[0].Error = "failed"
		case "stale":
			s.Panels[0].UpdatedAt = epoch.Format(time.RFC3339Nano)
		case "malformed":
			s.Panels[0].Content = "MEM=unknown"
		case "offline":
			s.Status = "ended"
		}
		e.Tick(epoch.Add(80*time.Second), []Session{s})
		if e.Snapshot().Incidents[0].Status != "firing" || len(e.Snapshot().Notices) != 1 {
			t.Fatalf("%s incorrectly recovered/reminded", mode)
		}
	}
}
func TestRepeatAcknowledgeSnoozeAndStopSemantics(t *testing.T) {
	e := memoryEngine()
	tick(e, 0, 95)
	tick(e, 11, 95)
	tick(e, 30, 95)
	if len(e.Snapshot().Notices) != 1 {
		t.Fatal("not deduplicated")
	}
	tick(e, 72, 95)
	if len(e.Snapshot().Notices) != 2 {
		t.Fatal("repeat missing")
	}
	id := e.Snapshot().Incidents[0].ID
	if err := e.Acknowledge(id); err != nil {
		t.Fatal(err)
	}
	tick(e, 140, 95)
	if len(e.Snapshot().Notices) != 2 {
		t.Fatal("acknowledged incident repeated")
	}
	e.Tick(epoch.Add(150*time.Second), nil)
	if e.Snapshot().Incidents[0].Status != "stopped" || len(e.Snapshot().Notices) != 2 {
		t.Fatal("manual disconnect claimed recovery")
	}
	e = memoryEngine()
	e.SnoozedUntil = epoch.Add(time.Minute)
	tick(e, 0, 95)
	tick(e, 11, 95)
	if len(e.Snapshot().Incidents) != 1 || len(e.Snapshot().Notices) != 0 {
		t.Fatal("snooze should keep incident but suppress notification")
	}
	tick(e, 61, 95)
	if len(e.Snapshot().Notices) != 1 {
		t.Fatal("ongoing problem should notify after snooze")
	}
}
func TestStaleWithoutUpdatesOfflineAndNormalizedLoad(t *testing.T) {
	p := DefaultPolicy()
	p.Rules = append(p.Rules, Rule{ID: "load", Name: "每核负载", Metric: "load", Enabled: true, Threshold: 1.5, Severity: "warning"})
	p.ConfirmSeconds = 0
	e := New(p)
	s := sample(epoch, 20)
	e.Tick(epoch, []Session{s})
	if len(e.Snapshot().Incidents) != 1 || e.Snapshot().Incidents[0].RuleID != "load" {
		t.Fatal("load not normalized by cores")
	}
	e.Tick(epoch.Add(31*time.Second), []Session{s})
	if len(e.Snapshot().Incidents) != 2 {
		t.Fatal("background timer did not detect stale data")
	}
	s.Status = "ended"
	e.Tick(epoch.Add(32*time.Second), []Session{s})
	if len(e.Snapshot().Incidents) != 3 {
		t.Fatal("unexpected disconnect not detected")
	}
	s = sample(epoch, 20)
	s.Panels[0].Content = "LOAD=999 999 999"
	e = New(p)
	e.Tick(epoch, []Session{s})
	if len(e.Snapshot().Incidents) != 0 {
		t.Fatal("load without core count must be unknown")
	}
}
func TestDisabledChangedRulesAndMissingPanels(t *testing.T) {
	e := memoryEngine()
	tick(e, 0, 95)
	tick(e, 11, 95)
	e.Policy.Rules[0].Threshold = 100
	tick(e, 12, 95)
	if e.Snapshot().Incidents[0].Status != "stopped" {
		t.Fatal("edited rule retained old incident")
	}
	e.Policy.Enabled = false
	tick(e, 13, 100)
	if len(e.Snapshot().Notices) != 1 {
		t.Fatal("disabled policy notified")
	}
	p := DefaultPolicy()
	p.ConfirmSeconds = 0
	p.Rules = []Rule{{ID: "keyword", Name: "服务异常", Metric: "keyword", Enabled: true, Severity: "critical", PanelID: "service", Keyword: "FAILED"}}
	e = New(p)
	s := sample(epoch, 1)
	e.Tick(epoch, []Session{s})
	if len(e.Snapshot().Incidents) != 0 {
		t.Fatal("missing custom panel fired")
	}
	s.Panels[0].ID = "service"
	s.Panels[0].Content = "service failed"
	e.Tick(epoch, []Session{s})
	if len(e.Snapshot().Incidents) != 1 {
		t.Fatal("keyword rule failed")
	}
}
func TestPolicyValidation(t *testing.T) {
	p := DefaultPolicy()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Rules = append(p.Rules, p.Rules[0])
	if p.Validate() == nil {
		t.Fatal("duplicate rules accepted")
	}
	p = DefaultPolicy()
	p.StaleSeconds = 1
	if p.Validate() == nil {
		t.Fatal("invalid stale interval accepted")
	}
	p = DefaultPolicy()
	p.Rules = []Rule{{ID: "memory", Name: "内存", Metric: "memory", Enabled: true, Severity: "warning", Threshold: 101}}
	if p.Validate() == nil {
		t.Fatal("invalid percentage accepted")
	}
}

func TestMonitorInventoryIncludesBuiltinsCommandsAndCollectionLifecycle(t *testing.T) {
	p := DefaultPolicy()
	p.Enabled = false // Disabling alerts must not hide what still collects.
	e := New(p)
	s := sample(epoch, 20)
	s.Panels[0].Command, s.Panels[0].Origin = "uptime; free -m", "system"
	s.Panels = append(s.Panels, credentialui.MonitorPanel{ID: "nginx", Title: "Nginx", Origin: "ai", Command: "systemctl is-active nginx", IntervalMS: 5000})
	e.Tick(epoch, []Session{s})
	monitors := e.Snapshot().Monitors
	if len(monitors) != 2 {
		t.Fatalf("missing built-in or custom monitor: %#v", monitors)
	}
	byID := func(id string) MonitorSummary {
		for _, m := range e.Snapshot().Monitors {
			if m.PanelID == id {
				return m
			}
		}
		t.Fatalf("missing monitor %s", id)
		return MonitorSummary{}
	}
	if m := byID("system"); m.Command != "uptime; free -m" || m.Origin != "system" || m.IntervalMS != 2000 || m.UpdatedAt != s.Panels[0].UpdatedAt || m.Status != "watching" {
		t.Fatalf("wrong live metadata: %#v", m)
	}
	if byID("nginx").Status != "waiting" {
		t.Fatal("never-collected monitor appears healthy")
	}
	s.Panels[1].Collecting = true
	e.Tick(epoch.Add(time.Second), []Session{s})
	if byID("nginx").Status != "collecting" {
		t.Fatal("collection state lost")
	}
	s.Panels[1].Collecting = false
	s.Panels[1].Error = "permission denied"
	e.Tick(epoch.Add(2*time.Second), []Session{s})
	if m := byID("nginx"); m.Status != "error" || m.Error != "permission denied" {
		t.Fatal("failed collection hidden")
	}
	e.Tick(epoch.Add(31*time.Second), []Session{s})
	if byID("system").Status != "stale" {
		t.Fatal("stale sample presented as current")
	}
	s.Status = "ended"
	e.Tick(epoch.Add(32*time.Second), []Session{s})
	for _, m := range e.Snapshot().Monitors {
		if m.Status != "stopped" {
			t.Fatal("ended session still appears active")
		}
	}
	if byID("nginx").Command != "systemctl is-active nginx" {
		t.Fatal("ended session lost its last configuration")
	}
	e.Tick(epoch.Add(33*time.Second), nil)
	if len(e.Snapshot().Monitors) != 0 {
		t.Fatal("removed session remains in inventory")
	}
}

func TestAIThresholdMonitorIsolationUnknownAndRemoval(t *testing.T) {
	p := DefaultPolicy()
	p.Rules = nil
	e := New(p)
	c := &panel.AlertCondition{Enabled: true, Kind: "above", Pattern: `(?m)^VALUE=([0-9.]+)$`, Threshold: 500, RecoveryThreshold: 400, Severity: "warning", ForSeconds: 0, RecoverySeconds: 0}
	s := sample(epoch, 0)
	s.Panels = []credentialui.MonitorPanel{{ID: "latency", Title: "API 耗时", UpdatedAt: epoch.Format(time.RFC3339Nano), Content: "fake display", SampleContent: "VALUE=650", IntervalMS: 5000, Alert: c}}
	other := sample(epoch, 0)
	other.ID = "other"
	e.Tick(epoch, []Session{s, other})
	if len(e.Snapshot().Incidents) != 1 || e.Snapshot().Incidents[0].SessionID != "s1" {
		t.Fatal("rule applied outside its own session")
	}
	s.Panels[0].SampleContent = "unknown"
	s.Panels[0].UpdatedAt = epoch.Add(time.Second).Format(time.RFC3339Nano)
	e.Tick(epoch.Add(time.Second), []Session{s})
	if e.Snapshot().Incidents[0].Status != "firing" || e.Snapshot().Monitors[0].Status != "unknown" {
		t.Fatal("failed numeric extraction recovered")
	}
	s.Panels[0].SampleContent = "VALUE=350"
	s.Panels[0].UpdatedAt = epoch.Add(2 * time.Second).Format(time.RFC3339Nano)
	e.Tick(epoch.Add(2*time.Second), []Session{s})
	if e.Snapshot().Incidents[0].Status != "recovered" {
		t.Fatal("numeric recovery missing")
	}
	s.Panels[0].SampleContent = "VALUE=650"
	s.Panels[0].UpdatedAt = epoch.Add(3 * time.Second).Format(time.RFC3339Nano)
	e.Tick(epoch.Add(3*time.Second), []Session{s})
	s.Panels = nil
	e.Tick(epoch.Add(4*time.Second), []Session{s})
	if e.Snapshot().Incidents[0].Status != "stopped" {
		t.Fatal("closing a monitor did not stop its incident")
	}
}
