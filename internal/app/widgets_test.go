package app

import (
	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
	"testing"
	"time"
)

func fixtureWidget(t *testing.T) (*App, *panel.Panel) {
	t.Helper()
	a := desktopTestApp(t)
	v := widgets.View{Kind: "custom", SDKVersion: 1, StateVersion: 1, Source: `import {defineWidget} from '@wrd/monitor-ui'; export default defineWidget({render({data}){return <p>{data?.n??'—'}</p>}})`}
	artifact, err := widgets.Compile(v)
	if err != nil {
		t.Fatal(err)
	}
	p := a.panels.Open(panel.Panel{ID: "test-widget", Origin: panel.OriginAI, Kind: panel.KindMonitor, Command: "printf 'n=1'", Interval: time.Second, SourceRevision: 1, Widget: &artifact, DataContract: &widgets.Contract{Format: "kv", Fields: []widgets.Field{{Name: "n", Type: "number", History: true}}}})
	return a, p
}
func TestWidgetCandidateAtomicCommitAndFreshData(t *testing.T) {
	a, p := fixtureWidget(t)
	action := agent.PanelAction{Action: "update", Panel: agent.PanelSpec{ID: p.ID, Title: "Updated", IntervalMS: 10000, ExpectedRevision: p.DefinitionRevision}}
	if _, _, err := a.executePanelAction(action); err != nil {
		t.Fatal(err)
	}
	c := a.widgetCandidates[p.ID]
	if c == nil || p.Title == "Updated" {
		t.Fatal("candidate mutated active state")
	}
	a.panels.UpdateContent(p.ID, "n=7", "实时", false)
	if err := a.finishWidget(p.ID, c.id, ""); err != nil {
		t.Fatal(err)
	}
	if p.Title != "Updated" || p.WidgetData.Data["n"] != float64(7) || len(p.WidgetData.History) != 1 {
		t.Fatalf("lost latest sample: %+v", p)
	}
	if p.WidgetData.IntervalMS != 10000 {
		t.Fatal("preserving samples lost the new sampling interval")
	}
	if _, _, err := a.executePanelAction(action); err == nil {
		t.Fatal("stale revision accepted")
	}
}
func TestWidgetPreviewFailureAndCloseRetainTruth(t *testing.T) {
	a, p := fixtureWidget(t)
	original := p.Command
	action := agent.PanelAction{Action: "update", Panel: agent.PanelSpec{ID: p.ID, Title: "failed layout", ExpectedRevision: p.DefinitionRevision}}
	if _, _, err := a.executePanelAction(action); err != nil {
		t.Fatal(err)
	}
	c := a.widgetCandidates[p.ID]
	if err := a.finishWidget(p.ID, c.id, "bad layout"); err != nil {
		t.Fatal(err)
	}
	if p.Command != original || p.SourceRevision != 1 {
		t.Fatal("failed preview altered collector")
	}
	if _, _, err := a.executePanelAction(action); err != nil {
		t.Fatal(err)
	}
	c = a.widgetCandidates[p.ID]
	if err := a.handleDesktopCommand(credentialui.WorkbenchCommand{Kind: "monitor-close", PanelID: p.ID, Deadline: time.Now().Add(time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if err := a.finishWidget(p.ID, c.id, ""); err == nil {
		t.Fatal("closed panel was resurrected")
	}
}
func TestWidgetRejectsOldSourceAndKeepsFailedSamplesUnknown(t *testing.T) {
	a, p := fixtureWidget(t)
	p.RunID = 4
	old := monitorResultEvent{panelID: p.ID, binding: p.BindingID, revision: p.SourceRevision, runID: 4, command: p.Command, content: "n=999"}
	if _, _, err := a.executePanelAction(agent.PanelAction{Action: "update", Panel: agent.PanelSpec{ID: p.ID, Command: "printf 'n=2'", ExpectedRevision: p.DefinitionRevision}}); err == nil {
		t.Fatal("legacy source edit bypassed v2 first capture")
	}
	// Simulate a restored legacy definition to exercise rejection of late v1
	// results. New AI source edits above are required to use the v2 transaction.
	p.SourceRevision += 2
	p.RunID = 5
	a.monitorActive[p.ID] = true
	a.applyMonitorResult(old)
	if p.Content != "" || !a.monitorActive[p.ID] {
		t.Fatal("old result contaminated new source or released new run")
	}
	a.applyMonitorResult(monitorResultEvent{panelID: p.ID, binding: p.BindingID, revision: p.SourceRevision, runID: 5, content: "n=3"})
	if p.WidgetData.Data["n"] != float64(3) {
		t.Fatal("new sample missing")
	}
	a.panels.UpdateContent(p.ID, "n=bad", "实时", false)
	if p.WidgetData.Data != nil || p.WidgetData.Error == "" || p.WidgetData.LastGood["n"] != float64(3) {
		t.Fatal("parse failure fabricated a live value")
	}
	if len(a.desktopState().WidgetTransfer.Data[p.ID].History) != 2 {
		t.Fatal("history unavailable without UI")
	}
}
