package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/alerts"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

const mysqlProcessCommand = "ps -C mysqld,mariadbd -o pid,comm,etime,pcpu,pmem,rss,nlwp,stat"

func TestCreateMySQLMonitorWithSafeIntegerComparison(t *testing.T) {
	command, err := os.ReadFile("../policy/testdata/mysql_widget_monitor.sh")
	if err != nil {
		t.Fatal(err)
	}
	a := desktopTestApp(t)
	action := agent.PanelAction{Action: "create", Panel: agent.PanelSpec{
		ID: "live-mysql", Purpose: "realtime", Type: "monitor", Title: "MySQL 状态",
		Command: string(command), IntervalMS: 5000,
	}}
	item, _, err := a.executePanelAction(action)
	if err != nil || item == nil {
		t.Fatalf("original MySQL collector could not be created: %v", err)
	}
	if item.Command != strings.TrimSpace(string(command)) || item.Interval != 5*time.Second {
		t.Fatal("policy normalization changed the actual collector")
	}
}

func mysqlPanelAction(operation string) agent.PanelAction {
	return agent.PanelAction{Action: operation, Panel: agent.PanelSpec{
		ID: "mysql-monitor", Type: "monitor", Title: "MySQL 进程监控", Collector: "process.top",
		Command: mysqlProcessCommand, IntervalMS: 15000, AllowAIRead: true,
	}}
}

func TestRealtimeAndAlertTasksHaveIndependentLifecycle(t *testing.T) {
	a := desktopTestApp(t)
	live := agent.PanelAction{Action: "create", Panel: agent.PanelSpec{ID: "live-nginx", Purpose: "realtime", Command: "ps -C nginx", IntervalMS: 2000}}
	alert := agent.PanelAction{Action: "create", Panel: agent.PanelSpec{ID: "alert-nginx", Purpose: "alert", Command: "systemctl is-active nginx", IntervalMS: 30000,
		Alert: &panel.AlertCondition{Enabled: true, Kind: "not_match", Pattern: "^active$", Severity: "critical"}}}
	for _, action := range []agent.PanelAction{live, alert} {
		if _, _, err := a.executePanelAction(action); err != nil {
			t.Fatal(err)
		}
	}
	state := a.desktopState()
	if len(state.Panels) != 2 || len(state.AlertMonitors) != 1 || state.AlertMonitors[0].ID != "alert-nginx" || state.AlertMonitors[0].IntervalMS != 30000 {
		t.Fatalf("collector inventories not separated: %+v", state)
	}
	for _, p := range state.Panels {
		if p.ID == "alert-nginx" || p.Alert != nil {
			t.Fatal("alert task leaked into real-time display")
		}
	}
	last := time.Now()
	a.monitorLast[live.Panel.ID], a.monitorLast[alert.Panel.ID] = last, last
	command := func(kind, id string) error {
		return a.handleDesktopCommand(credentialui.WorkbenchCommand{Kind: kind, PanelID: id, Deadline: time.Now().Add(time.Second).UnixMilli()})
	}
	for _, op := range []struct{ kind, id string }{{"monitor-close", alert.Panel.ID}, {"monitor-refresh", alert.Panel.ID}, {"alert-close", live.Panel.ID}, {"alert-refresh", live.Panel.ID}} {
		if command(op.kind, op.id) == nil {
			t.Fatalf("cross-purpose operation accepted: %+v", op)
		}
	}
	if err := command("alert-refresh", alert.Panel.ID); err != nil || !a.monitorLast[alert.Panel.ID].IsZero() || !a.monitorLast[live.Panel.ID].Equal(last) {
		t.Fatalf("alert refresh affected real-time schedule: %v", err)
	}
	if err := command("monitor-close", live.Panel.ID); err != nil || a.panels.ByID(alert.Panel.ID) == nil {
		t.Fatalf("closing display stopped alert task: %v", err)
	}
	if err := command("alert-close", alert.Panel.ID); err != nil || len(a.desktopState().AlertMonitors) != 0 || a.panels.ByID("system") == nil {
		t.Fatalf("alert removal affected display inventory: %v", err)
	}
}

func TestPanelActionsRejectMixedPurposesAtomically(t *testing.T) {
	a := desktopTestApp(t)
	condition := &panel.AlertCondition{Enabled: true, Kind: "match", Pattern: "failed", Severity: "warning"}
	for _, spec := range []agent.PanelSpec{
		{ID: "mixed", Purpose: "realtime", Command: "uptime", Alert: condition},
		{ID: "implicit", Command: "uptime", Alert: condition},
		{ID: "missing-rule", Purpose: "alert", Command: "uptime"},
		{ID: "bad-purpose", Purpose: "both", Command: "uptime"},
	} {
		if _, _, err := a.executePanelAction(agent.PanelAction{Action: "create", Panel: spec}); err == nil || a.panels.ByID(spec.ID) != nil {
			t.Fatalf("mixed or incomplete configuration accepted: %+v", spec)
		}
	}
	action := mysqlPanelAction("create")
	item, _, err := a.executePanelAction(action)
	if err != nil {
		t.Fatal(err)
	}
	action.Panel.Purpose, action.Panel.Alert = "alert", condition
	if _, _, err := a.executePanelAction(action); err == nil || item.IsAlert() || item.Alert != nil {
		t.Fatal("existing realtime collector converted to alert task")
	}
	// A full display inventory must not consume the independent alert quota.
	for a.panels.CountPurpose(false) < 16 {
		a.panels.Open(panel.Panel{Command: "uptime"})
	}
	action.Panel.ID = "independent-alert"
	if _, _, err := a.executePanelAction(action); err != nil {
		t.Fatalf("display quota blocked alert task: %v", err)
	}
}

func TestMySQLPanelUsesExplicitCommandAndRecoversFailedCreation(t *testing.T) {
	a := desktopTestApp(t)
	rejected := mysqlPanelAction("create")
	rejected.Panel.Command = "systemctl restart mysql"
	a.applyPanelAction(rejected)
	if a.panels.ByID("mysql-monitor") != nil {
		t.Fatal("unsafe command was hidden by the safe collector")
	}
	if !strings.Contains(a.chat[len(a.chat)-1].Content, "systemctl restart") {
		t.Fatal("rejection omitted the policy reason")
	}
	// Reproduce the follow-up: update after creation was refused.
	a.applyPanelAction(mysqlPanelAction("update"))
	item := a.panels.ByID("mysql-monitor")
	if item == nil || item.Command != mysqlProcessCommand || item.Interval != 15*time.Second {
		t.Fatalf("MySQL configuration did not take effect: %#v", item)
	}
	a.applyPanelAction(mysqlPanelAction("open"))
	if len(a.panels.Panels()) != 2 || a.panels.ByID(item.ID) != item {
		t.Fatal("reopening created a duplicate panel")
	}
}

func TestPanelConfigurationUpdateIsAtomicAndDiscardsOldResult(t *testing.T) {
	a := desktopTestApp(t)
	a.applyPanelAction(mysqlPanelAction("create"))
	item := a.panels.ByID("mysql-monitor")
	a.panels.UpdateContent(item.ID, "old output", "live", false)
	a.monitorLast[item.ID] = time.Now()
	a.monitorActive[item.ID] = true
	update := mysqlPanelAction("update")
	update.Panel.Command = "systemctl restart mysql"
	update.Panel.Title = "changed"
	update.Panel.IntervalMS = 5000
	a.applyPanelAction(update)
	if item.Command != mysqlProcessCommand || item.Title == "changed" || item.Content != "old output" || item.Interval != 15*time.Second {
		t.Fatal("rejected update partially changed the live panel")
	}
	update.Panel.Command = "ps -C mysqld -o pid,pcpu,rss"
	a.applyPanelAction(update)
	if item.Command != update.Panel.Command || item.Title != "changed" || item.Interval != 5*time.Second || item.Content != "" || !a.monitorLast[item.ID].IsZero() {
		t.Fatalf("configuration update not applied: %#v", item)
	}
	a.applyMonitorResult(monitorResultEvent{panelID: item.ID, command: mysqlProcessCommand, content: "stale output", err: fmt.Errorf("stale failure")})
	if item.Content != "" || item.LastError != "" || a.monitorActive[item.ID] || !a.monitorLast[item.ID].IsZero() {
		t.Fatal("old query result overwrote the new configuration")
	}
	a.applyMonitorResult(monitorResultEvent{panelID: item.ID, command: item.Command, content: "new output"})
	if item.Content != "new output" || item.RefreshCount != 1 {
		t.Fatal("new query result was discarded")
	}
}

func TestPanelActionsRejectInvalidConfigurationAndProtectUserPanels(t *testing.T) {
	a := desktopTestApp(t)
	a.panels.Open(panel.Panel{ID: "user", Origin: panel.OriginUser, Command: "uptime"})
	for _, action := range []agent.PanelAction{
		{Action: "update", Panel: agent.PanelSpec{ID: "missing"}},
		{Action: "update", Panel: agent.PanelSpec{Command: "uptime"}},
		{Action: "append", Panel: agent.PanelSpec{ID: "missing", Content: "text"}},
		{Action: "close", Panel: agent.PanelSpec{ID: "missing"}},
		{Action: "create", Panel: agent.PanelSpec{ID: "unknown", Collector: "unknown"}},
		{Action: "create", Panel: agent.PanelSpec{ID: "empty"}},
		{Action: "update", Panel: agent.PanelSpec{ID: "user", Command: "ps aux"}},
		{Action: "open", Panel: agent.PanelSpec{ID: "system", Command: "ps aux"}},
		{Action: "append", Panel: agent.PanelSpec{ID: "user", Content: "text"}},
		{Action: "close", Panel: agent.PanelSpec{ID: "user"}},
	} {
		a.applyPanelAction(action)
		if a.chat[len(a.chat)-1].Role != "error" {
			t.Errorf("missing actionable error for %#v", action)
		}
	}
	if len(a.panels.Panels()) != 2 || a.panels.ByID("user").Command != "uptime" || a.panels.ByID("user").Content != "" {
		t.Fatal("invalid operation changed panels")
	}
}

type panelContextProvider struct {
	approvalTestProvider
	requests chan agent.Request
}

func (p *panelContextProvider) Stream(_ context.Context, request agent.Request, _ agent.Callbacks) (agent.Turn, error) {
	p.requests <- request
	return agent.Turn{Type: "finished"}, nil
}

func TestPanelResultsReachNextAIRequestWithActualState(t *testing.T) {
	a := desktopTestApp(t)
	a.agentSessionID = "existing-session"
	a.applyPanelAction(mysqlPanelAction("open"))
	rejected := mysqlPanelAction("update")
	rejected.Panel.Command = "printf super-secret > /tmp/report"
	a.applyPanelAction(rejected)
	p := &panelContextProvider{requests: make(chan agent.Request, 1)}
	a.agent = p
	a.startAI("为啥没改成功", false)
	select {
	case request := <-p.requests:
		if !strings.Contains(request.SystemContext["monitor_panel_result_1"], "只读策略拒绝") {
			t.Fatal("AI did not receive the rejection")
		}
		var state map[string]any
		if err := json.Unmarshal([]byte(request.SystemContext["monitor_panel_state_1"]), &state); err != nil {
			t.Fatal(err)
		}
		if state["command"] != mysqlProcessCommand || state["intervalMs"] != float64(15000) {
			t.Fatalf("AI received wrong live configuration: %#v", state)
		}
		for _, value := range request.SystemContext {
			if strings.Contains(value, "super-secret") {
				t.Fatal("panel context leaked saved credentials")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("AI request not sent")
	}
	// Let the in-flight request finish before cleanup.
	select {
	case event := <-a.events:
		a.handleEvent(event)
	case <-time.After(time.Second):
		t.Fatal("AI request did not finish")
	}
}

func TestAllAdvertisedCollectorsCreatePanels(t *testing.T) {
	for _, collector := range []string{"system.overview", "process.top", "disk.usage", "network.sockets", "service.status"} {
		t.Run(collector, func(t *testing.T) {
			a := desktopTestApp(t)
			a.applyPanelAction(agent.PanelAction{Action: "create", Panel: agent.PanelSpec{ID: "builtin", Collector: collector}})
			if a.panels.ByID("builtin") == nil {
				t.Fatalf("builtin collector refused: %#v", a.chat)
			}
		})
	}
}

func TestAICustomMonitorFlowsThroughDesktopToPetAlerts(t *testing.T) {
	a := desktopTestApp(t)
	action := agent.PanelAction{Action: "create", Panel: agent.PanelSpec{
		ID: "nginx", Purpose: "alert", Type: "monitor", Title: "Nginx 服务", Command: "systemctl is-active nginx 2>/dev/null || true", IntervalMS: 5000, AllowAIRead: true,
		Alert: &panel.AlertCondition{Enabled: true, Kind: "not_match", Pattern: `(?m)^active\s*$`, Severity: "critical", ForSeconds: 0, RecoverySeconds: 0},
	}}
	a.applyPanelAction(action)
	item := a.panels.ByID("nginx")
	if item == nil || item.Alert == nil {
		t.Fatalf("AI monitor failed: %#v", a.chat)
	}
	if !strings.Contains(a.chat[len(a.chat)-1].Content, "已配置异常提醒") {
		t.Fatal("creation receipt omitted alert")
	}
	// AI-written text has no collector timestamp/sample and cannot manufacture a
	// failure or recovery. Only completed read-only collection is evaluated.
	a.applyPanelAction(agent.PanelAction{Action: "append", Panel: agent.PanelSpec{ID: "nginx", Content: "inactive"}})
	p := alerts.DefaultPolicy()
	p.Rules = nil
	e := alerts.New(p)
	feed := func() {
		e.Tick(time.Now(), []alerts.Session{{ID: "session", Name: "host", Status: "running", CreatedAt: time.Now(), Panels: a.desktopState().AlertMonitors}})
	}
	feed()
	if len(e.Snapshot().Incidents) != 0 {
		t.Fatal("AI fabricated an alert from display text")
	}
	a.panels.UpdateContent("nginx", "inactive", "live", false)
	feed()
	state := e.Snapshot()
	if len(state.Notices) != 1 || state.Notices[0].Incident.SessionID != "session" || state.Notices[0].Incident.Severity != "critical" {
		t.Fatalf("custom monitor did not reach alert/companion feed: %#v", state)
	}
	a.applyPanelAction(agent.PanelAction{Action: "append", Panel: agent.PanelSpec{ID: "nginx", Content: "active"}})
	feed()
	if e.Snapshot().Incidents[0].Status != "firing" {
		t.Fatal("AI display text fabricated recovery")
	}
	a.panels.UpdateContent("nginx", "active", "live", false)
	feed()
	if e.Snapshot().Notices[1].Kind != "recovered" {
		t.Fatal("fresh service recovery did not reach companion")
	}
	// Invalid alert updates are atomic, just like rejected collector commands.
	action.Action = "update"
	action.Panel.Title = "bad"
	action.Panel.Alert.Pattern = "["
	a.applyPanelAction(action)
	if item.Title == "bad" || item.Alert.Pattern == "[" {
		t.Fatal("invalid alert partially updated monitor")
	}
}
