package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSummaryProvidersPreserveInstructionsAndLongSource(t *testing.T) {
	source := "source-start\n" + strings.Repeat("历史资料。", 6000) + "\nsource-end"
	request := NewSummaryRequest(source)
	checkPrompt := func(t *testing.T, prompt string) {
		t.Helper()
		for _, want := range []string{source, "会话资料总结助手", "信息不足时写入待确认事项", "type 必须是 finished", "proposal 必须为 null", "panelActions 必须为空数组"} {
			if !strings.Contains(prompt, want) {
				t.Fatal("summary instructions or source were lost")
			}
		}
		for _, unwanted := range []string{"正在本机包装一个真实的 OpenSSH 会话", "如果信息不足，返回 ask_user", "常用只读 collector"} {
			if strings.Contains(prompt, unwanted) {
				t.Fatalf("summary inherited chat instructions: %s", unwanted)
			}
		}
	}
	response := `{"type":"finished","message":"已生成总结","proposal":null,"panelActions":[]}`
	for _, name := range []string{"codex", "claude", "model"} {
		t.Run(name, func(t *testing.T) {
			var provider Provider
			switch name {
			case "codex":
				p := testCodexProvider(t, response)
				p.run = func(_ context.Context, _ string, _ []string, prompt string) ([]byte, []byte, error) {
					checkPrompt(t, prompt)
					return []byte("{\"type\":\"thread.started\",\"thread_id\":\"summary\"}\n" +
						`{"type":"item.completed","item":{"type":"agent_message","text":` + string(mustJSON(response)) + "}}\n"), nil, nil
				}
				provider = p
			case "claude":
				p := testClaudeProvider(t, response)
				p.run = func(_ context.Context, _ string, _ []string, _, prompt string) ([]byte, []byte, error) {
					checkPrompt(t, prompt)
					return claudeResultJSON("summary", response), nil, nil
				}
				provider = p
			case "model":
				p := NewDirectModelProvider("model", "https://model.example", "", "test", false)
				p.HTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					var body directChatRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if len(body.Messages) != 1 {
						t.Fatalf("unexpected messages: %d", len(body.Messages))
					}
					checkPrompt(t, body.Messages[0].Content)
					data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": response}}}})
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
				})}
				provider = p
			}
			turn, err := provider.Stream(context.Background(), request, Callbacks{})
			if err != nil || turn.Type != "finished" || turn.Message != "已生成总结" {
				t.Fatalf("summary: %+v %v", turn, err)
			}
		})
	}
	// HTTP agents receive the complete instructions in Request.Prompt, too.
	checkPrompt(t, request.Prompt)
	checkPrompt(t, buildCodexContinuationPrompt(request, "truncated input"))
}
