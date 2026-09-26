package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

const mysqlProcessCommand = "ps -C mysqld,mariadbd -o pid,comm,etime,pcpu,pmem,rss,nlwp,stat"

func mysqlPanelAction(operation string) agent.PanelAction {
	return agent.PanelAction{Action: operation, Panel: agent.PanelSpec{
		ID: "mysql-monitor", Type: "monitor", Title: "MySQL 进程监控", Collector: "process.top",
		Command: mysqlProcessCommand, IntervalMS: 15000, AllowAIRead: true,
	}}
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
