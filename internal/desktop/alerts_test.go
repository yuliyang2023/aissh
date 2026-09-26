package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/alerts"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

func TestMonitoringPersistsValidatedPolicyAndTicksWithoutFrontend(t *testing.T) {
	dir := t.TempDir()
	s := New(catalog.New(dir, memorySecrets{}))
	p := alerts.DefaultPolicy()
	p.ConfirmSeconds = 0
	p.Rules = p.Rules[:1]
	if err := s.SaveAlertPolicy(p); err != nil {
		t.Fatal(err)
	}
	s = New(catalog.New(dir, memorySecrets{}))
	if s.GetMonitoring().Policy.ConfirmSeconds != 0 {
		t.Fatal("policy did not persist")
	}
	bad := p
	bad.RepeatMinutes = 0
	if s.SaveAlertPolicy(bad) == nil {
		t.Fatal("invalid policy accepted")
	}
	now := time.Now()
	s.sessions["one"] = &terminalSession{id: "one", info: SessionInfo{Name: "host", Status: "running", CreatedAt: now.Format(time.RFC3339Nano)}, workbench: &credentialui.WorkbenchState{AlertMonitors: []credentialui.MonitorPanel{{ID: "alert-nginx", Error: "collector failed", UpdatedAt: now.Format(time.RFC3339Nano)}}}}
	s.tickAlerts(now)
	state := s.GetMonitoring()
	if state.LiveSessions != 1 || len(state.Incidents) != 1 {
		t.Fatal("backend did not evaluate session")
	}
	if err := s.AcknowledgeAlert(state.Incidents[0].ID); err != nil {
		t.Fatal(err)
	}
	if !s.GetMonitoring().Incidents[0].Acknowledged {
		t.Fatal("ack not reflected")
	}
	if err := s.CloseSession("one"); err != nil {
		t.Fatal(err)
	}
	s.tickAlerts(now.Add(time.Second))
	if s.GetMonitoring().Incidents[0].Status != "stopped" {
		t.Fatal("manual disconnect not stopped")
	}
	if err := os.WriteFile(filepath.Join(dir, "monitor-alerts.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if New(catalog.New(dir, memorySecrets{})).GetMonitoring().Error == "" {
		t.Fatal("corrupt config silently ignored")
	}
}

func TestRealtimePanelsNeverFeedPetAlerts(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	s.alerts.Policy.ConfirmSeconds = 0
	now := time.Now()
	condition := &panel.AlertCondition{Enabled: true, Kind: "match", Pattern: "failed", Severity: "critical"}
	live := credentialui.MonitorPanel{ID: "system", Content: "MEM=99/100 MB (99%)", SampleContent: "failed", Error: "failed", Alert: condition, UpdatedAt: now.Format(time.RFC3339Nano)}
	session := &terminalSession{id: "one", info: SessionInfo{Name: "host", Status: "running", CreatedAt: now.Format(time.RFC3339Nano)}, workbench: &credentialui.WorkbenchState{Panels: []credentialui.MonitorPanel{live}}}
	s.sessions["one"] = session
	for _, status := range []string{"running", "ended"} {
		session.info.Status = status
		s.tickAlerts(now.Add(time.Minute))
		state := s.GetMonitoring()
		if state.LiveSessions != 0 || len(state.Monitors) != 0 || len(state.Incidents) != 0 || len(state.Notices) != 0 {
			t.Fatalf("display-only session created pet monitoring state: %+v", state)
		}
	}
	session.info.Status = "running"
	session.workbench.AlertMonitors = []credentialui.MonitorPanel{{ID: "alert-nginx", Title: "Nginx", SampleContent: "active", Alert: condition, UpdatedAt: now.Format(time.RFC3339Nano), IntervalMS: 5000}}
	s.tickAlerts(now)
	if state := s.GetMonitoring(); state.LiveSessions != 1 || len(state.Monitors) != 1 || len(state.Incidents) != 0 {
		t.Fatalf("real-time errors leaked into healthy alert task: %+v", state)
	}
	session.workbench.AlertMonitors[0].SampleContent = "failed"
	session.workbench.AlertMonitors[0].UpdatedAt = now.Add(time.Second).Format(time.RFC3339Nano)
	s.tickAlerts(now.Add(time.Second))
	if state := s.GetMonitoring(); len(state.Incidents) != 1 || state.Incidents[0].RuleID != "panel:alert-nginx" {
		t.Fatalf("independent alert sample did not trigger: %+v", state)
	}
	session.workbench.AlertMonitors = nil
	s.tickAlerts(now.Add(2 * time.Second))
	if state := s.GetMonitoring(); len(state.Monitors) != 0 || state.Incidents[0].Status != "stopped" || len(state.Notices) != 1 {
		t.Fatalf("removal must stop monitoring without fabricating recovery: %+v", state)
	}
}

func TestLegacyDisplayThresholdsRetiredOnLoad(t *testing.T) {
	dir := t.TempDir()
	p := alerts.DefaultPolicy()
	p.RepeatMinutes = 12
	p.Rules = append(p.Rules, alerts.Rule{ID: "memory", Name: "内存", Metric: "memory", Enabled: true, Threshold: 90, Severity: "warning"})
	if err := saveAlertPolicy(dir, p); err != nil {
		t.Fatal(err)
	}
	s := New(catalog.New(dir, memorySecrets{}))
	state := s.GetMonitoring()
	if len(state.Policy.Rules) != 3 || state.Policy.RepeatMinutes != 12 {
		t.Fatalf("legacy migration lost notification preferences: %+v", state.Policy)
	}
	for _, r := range state.Policy.Rules {
		if r.Metric == "memory" {
			t.Fatal("legacy display threshold survived migration")
		}
	}
}

func TestPetChannelAuthenticationActionsAndLimitedPayload(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	handler := s.petHandler("secret")
	request := func(method, path, token, origin, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if request("GET", "/state", "", "", "").Code != 401 {
		t.Fatal("missing capability accepted")
	}
	if request("GET", "/state", "Bearer secret", "https://example.com", "").Code != 401 {
		t.Fatal("browser origin accepted")
	}
	w := request("GET", "/state", "Bearer secret", "", "")
	var state alerts.State
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &state) != nil || state.Policy.Rules != nil {
		t.Fatal("pet payload not limited")
	}
	if request("POST", "/action", "Bearer secret", "", `{"kind":"approve"}`).Code != 400 {
		t.Fatal("pet can execute unknown action")
	}
	if request("POST", "/action", "Bearer secret", "", `{"kind":"snooze"}`).Code != http.StatusNoContent || !s.GetMonitoring().SnoozedUntil.After(time.Now()) {
		t.Fatal("snooze failed")
	}
	if request("POST", "/action", "Bearer secret", "", `{"kind":"resume"}`).Code != http.StatusNoContent || !s.GetMonitoring().SnoozedUntil.IsZero() {
		t.Fatal("resume failed")
	}
	var event string
	s.emit = func(name string, args ...any) { event = name }
	if request("POST", "/action", "Bearer secret", "", `{"kind":"open"}`).Code != http.StatusNoContent || event != "desktop-command" {
		t.Fatal("main window navigation failed")
	}
}
