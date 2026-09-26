package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestModelCanCreateCustomMonitorAndAlert(t *testing.T) {
	provider := testCodexProvider(t, `{"type":"finished","message":"已请求创建 nginx 监控。","proposal":null,"panelActions":[{"action":"create","panel":{"id":"nginx","purpose":"alert","type":"monitor","title":"nginx 服务","collector":"","command":"systemctl is-active nginx 2>/dev/null || true","intervalMs":5000,"content":"","allowAIRead":true,"alert":{"enabled":true,"kind":"not_match","pattern":"(?m)^active\\s*$","threshold":0,"recoveryThreshold":0,"severity":"critical","forSeconds":10,"recoverySeconds":10}}}]}`)
	var action PanelAction
	_, err := provider.Stream(context.Background(), Request{Event: "start", Prompt: "监控 nginx，停止时提醒"}, Callbacks{OnPanelAction: func(a PanelAction) { action = a }})
	if err != nil {
		t.Fatal(err)
	}
	if action.Panel.Purpose != "alert" || action.Panel.Alert == nil || action.Panel.Alert.Validate() != nil || action.Panel.Alert.Kind != "not_match" || action.Panel.Command == "" {
		t.Fatalf("monitor/alert was lost in model response: %#v", action)
	}
	var schema any
	if json.Unmarshal([]byte(codexResponseSchema), &schema) != nil {
		t.Fatal("response schema is not valid JSON")
	}
	for _, prompt := range []string{buildCodexPrompt(Request{}, nil, "创建监控"), buildCodexContinuationPrompt(Request{}, "创建监控")} {
		if !strings.Contains(prompt, "purpose=realtime") || !strings.Contains(prompt, "purpose=alert") || !strings.Contains(prompt, "panel.alert") || !strings.Contains(prompt, "recoveryThreshold") {
			t.Fatal("model missing dynamic monitor protocol")
		}
	}
}
