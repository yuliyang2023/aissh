package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMessageStreamEveryChunkBoundary(t *testing.T) {
	input := `{"proposal":{"message":"hidden","command":"echo secret"},"message":"中文\n引号\"与反斜线\\和\ud83d\ude80🚀","panelActions":[]}`
	want := "中文\n引号\"与反斜线\\和🚀🚀"
	for size := 1; size <= len(input); size++ {
		var got strings.Builder
		stream := messageStream{emit: func(s string) { got.WriteString(s) }}
		for i := 0; i < len(input); i += size {
			stream.Write(input[i:min(i+size, len(input))])
		}
		if got.String() != want {
			t.Fatalf("chunk %d: %q", size, got.String())
		}
	}
}

func TestDirectModelDeliversBeforeResponseCompletes(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	provider := NewDirectModelProvider("test", "https://example.invalid", "", "test", false)
	provider.Streaming = true
	provider.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body directChatRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || !body.Stream {
			t.Errorf("not a streaming request: %#v %v", body, err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}, nil
	})}
	write := func(delta string, finish any) {
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": delta}, "finish_reason": finish}}})
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", data)
	}
	got := make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		turn, err := provider.Stream(context.Background(), Request{Event: "start", Prompt: "check"}, Callbacks{OnAnswer: func(s string) { got <- s }})
		if err == nil && turn.Message != "实时回复" {
			err = fmt.Errorf("turn=%#v", turn)
		}
		done <- err
	}()
	go write(`{"message":"实时`, nil)
	select {
	case value := <-got:
		if value != "实时" {
			t.Fatal(value)
		}
	case <-time.After(time.Second):
		t.Fatal("buffered until completion")
	}
	go func() {
		write(`回复","type":"finished","proposal":null,"panelActions":[]}`, "stop")
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("did not finish")
	}
}

func TestDirectModelRejectsTruncatedStream(t *testing.T) {
	p := NewDirectModelProvider("test", "", "secret", "", false)
	for _, input := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"message\\\":\\\"partial\"}}]}\n\n",
		"data: {\"choices\":[{\"finish_reason\":\"length\"}]}\n\n",
		"data: {\"error\":{\"message\":\"bad secret\"}}\n\n",
	} {
		if _, err := p.readChatStream(strings.NewReader(input), Callbacks{}); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("err=%v", err)
		}
	}
}

func TestClaudeStructuredOutputDeltas(t *testing.T) {
	var data strings.Builder
	write := func(v any) { b, _ := json.Marshal(v); data.Write(b); data.WriteByte('\n') }
	write(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]string{"type": "tool_use", "name": "StructuredOutput"}}})
	for _, part := range []string{`{"message":"实时`, `回复","type":"finished","proposal":null,"panelActions":[]}`} {
		write(map[string]any{"type": "stream_event", "event": map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]string{"type": "input_json_delta", "partial_json": part}}})
	}
	data.Write(claudeResultJSON("c1", `{"message":"实时回复","type":"finished","proposal":null,"panelActions":[]}`))
	var got string
	result, err := readClaudeStream(strings.NewReader(data.String()), Callbacks{OnAnswer: func(s string) { got += s }})
	if err != nil || got != "实时回复" || len(result) == 0 {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestCodexAppServerHelper(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "aissh-stream-helper" {
		return
	}
	defer os.Exit(0)
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	turn := 0
	for {
		var request struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
			Result map[string]string          `json:"result"`
		}
		if decoder.Decode(&request) != nil {
			return
		}
		reply := func(result any) { _ = encoder.Encode(map[string]any{"id": request.ID, "result": result}) }
		switch request.Method {
		case "initialize":
			reply(map[string]string{"userAgent": "fixture"})
		case "initialized":
		case "thread/start", "thread/resume":
			if string(request.Params["approvalPolicy"]) != `"never"` {
				os.Exit(3)
			}
			reply(map[string]any{"thread": map[string]string{"id": "fixture-thread"}})
		case "turn/start":
			turn++
			if !strings.Contains(string(request.Params["sandboxPolicy"]), `"networkAccess":false`) || len(request.Params["outputSchema"]) == 0 {
				os.Exit(4)
			}
			turnID := fmt.Sprintf("turn-%d", turn)
			notify := func(method string, params map[string]any) {
				params["threadId"] = "fixture-thread"
				params["turnId"] = turnID
				_ = encoder.Encode(map[string]any{"method": method, "params": params})
			}
			reply(map[string]any{"turn": map[string]string{"id": turnID}})
			if strings.Contains(string(request.Params["input"]), "hang-test") {
				continue
			}
			if strings.Contains(string(request.Params["input"]), "disconnect-test") {
				return
			}
			notify("item/agentMessage/delta", map[string]any{"itemId": "text", "delta": `{"message":"实时`})
			notify("item/started", map[string]any{"item": map[string]string{"id": "cmd", "type": "commandExecution", "command": "cat README.md"}})
			notify("item/commandExecution/outputDelta", map[string]any{"itemId": "cmd", "delta": "file content"})
			_ = encoder.Encode(map[string]any{"id": "approval", "method": "item/commandExecution/requestApproval", "params": map[string]string{}})
			if decoder.Decode(&request) != nil || request.Result["decision"] != "decline" {
				os.Exit(5)
			}
			notify("item/completed", map[string]any{"item": map[string]any{"id": "cmd", "type": "commandExecution", "command": "cat README.md", "status": "completed", "exitCode": 0, "aggregatedOutput": "file content"}})
			notify("item/agentMessage/delta", map[string]any{"itemId": "text", "delta": `回复","type":"finished","proposal":null,"panelActions":[]}`})
			notify("item/completed", map[string]any{"item": map[string]string{"id": "text", "type": "agentMessage", "phase": "final_answer", "text": `{"message":"实时回复","type":"finished","proposal":null,"panelActions":[]}`}})
			notify("turn/completed", map[string]any{"turn": map[string]string{"status": "completed"}})
		}
	}
}

func streamingCodexFixture(t *testing.T) (*CodexProvider, *int) {
	t.Helper()
	p := NewCodexProvider(os.Args[0], "")
	p.Streaming = true
	p.Timeout = time.Second
	starts := 0
	p.startServer = func(string) (*codexAppServer, error) {
		starts++
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCodexAppServerHelper$", "--", "aissh-stream-helper")
		return startCodexAppServerCommand(ctx, cancel, cmd)
	}
	p.run = func(context.Context, string, []string, string) ([]byte, []byte, error) { return nil, nil, nil }
	t.Cleanup(func() { _ = p.Close() })
	return p, &starts
}

func TestCodexAppServerStreamsAndReusesProcess(t *testing.T) {
	p, starts := streamingCodexFixture(t)
	request := Request{Event: "start", Prompt: "check"}
	ids := map[string]bool{}
	for i := 0; i < 2; i++ {
		var text string
		var updates int
		turn, err := p.Stream(context.Background(), request, Callbacks{
			OnAnswer: func(s string) { text += s },
			OnActivity: func(a Activity) {
				if text == "" {
					t.Error("activity arrived before first text delta")
				}
				ids[a.ID] = true
				updates++
			},
		})
		if err != nil || text != "实时回复" || turn.Message != text || updates != 3 {
			t.Fatalf("turn=%#v text=%q updates=%d err=%v", turn, text, updates, err)
		}
		request = Request{Event: "user_message", SessionID: turn.Session.ID, Prompt: "continue"}
	}
	if *starts != 1 || len(ids) != 2 {
		t.Fatalf("starts=%d ids=%v", *starts, ids)
	}
}

func TestCodexAppServerDisconnectAndTimeoutResetProcess(t *testing.T) {
	for _, prompt := range []string{"disconnect-test", "hang-test"} {
		t.Run(prompt, func(t *testing.T) {
			p, _ := streamingCodexFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if _, err := p.Stream(ctx, Request{Event: "start", Prompt: prompt}, Callbacks{}); err == nil {
				t.Fatal("unfinished turn succeeded")
			}
			if p.server != nil {
				t.Fatal("broken process retained")
			}
		})
	}
}
