package desktop

import (
	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
	"testing"
)

func TestWidgetCacheScopeAndCleanup(t *testing.T) {
	s := &terminalSession{}
	v := widgets.View{Kind: "custom", SDKVersion: 1, StateVersion: 1, Source: `import {defineWidget} from '@wrd/monitor-ui';export default defineWidget({render(){return <p/>}})`}
	a, err := widgets.Compile(v)
	if err != nil {
		t.Fatal(err)
	}
	input := credentialui.WorkbenchExchange{State: credentialui.WorkbenchState{Panels: []credentialui.MonitorPanel{{ID: "p", Widget: &widgets.Meta{Hash: a.Hash, BindingID: "b", Revision: 1}}}}, Widgets: widgets.Transfer{Artifacts: []widgets.Artifact{a}, Data: map[string]widgets.Data{"p": {BindingID: "b", Sequence: 1}}}}
	if err = acceptWidgets(s, input); err != nil {
		t.Fatal(err)
	}
	input.Widgets.Data["p"] = widgets.Data{BindingID: "other", Sequence: 9}
	if err = acceptWidgets(s, input); err == nil {
		t.Fatal("cross-binding data accepted")
	}
	if s.widgetData["p"].Sequence != 1 {
		t.Fatal("partial mutation")
	}
	if err = acceptWidgets(s, credentialui.WorkbenchExchange{}); err != nil {
		t.Fatal(err)
	}
	if len(s.widgetArtifacts) != 0 || len(s.widgetData) != 0 {
		t.Fatal("closed components retained")
	}
}
