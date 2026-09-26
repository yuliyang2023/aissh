package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type codexRPCMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// One stdio process per SSH workspace. Calls are serialized by streamMu, while
// stdout is always drained. No listening socket or shared desktop daemon is used.
type codexAppServer struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	output   io.ReadCloser
	messages chan codexRPCMessage
	done     chan struct{}
	stop     context.CancelFunc
	readErr  error
	nextID   int
}

func startCodexAppServer(path string) (*codexAppServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	args := []string{"app-server", "--listen", "stdio://"}
	for _, feature := range []string{"plugins", "remote_plugin", "apps", "browser_use", "computer_use", "image_generation", "multi_agent", "hooks"} {
		args = append(args, "--disable", feature)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = localProviderEnvironment("codex", path)
	cmd.Dir = codexWorkingDirectory("")
	return startCodexAppServerCommand(ctx, cancel, cmd)
}

func startCodexAppServerCommand(ctx context.Context, cancel context.CancelFunc, cmd *exec.Cmd) (*codexAppServer, error) {
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		cancel()
		return nil, err
	}
	// Protocol errors arrive on stdout. Do not expose arbitrary process logs.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		cancel()
		return nil, err
	}
	s := &codexAppServer{cmd: cmd, input: input, output: output, messages: make(chan codexRPCMessage, 64), done: make(chan struct{}), stop: cancel}
	go func() {
		defer close(s.done)
		defer close(s.messages)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for scanner.Scan() {
			var message codexRPCMessage
			if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
				s.readErr = err
				return
			}
			select {
			case s.messages <- message:
			case <-ctx.Done():
				return
			}
		}
		s.readErr = scanner.Err()
	}()
	return s, nil
}

func (s *codexAppServer) close() {
	s.stop()
	_ = s.input.Close()
	_ = s.output.Close()
	<-s.done
	_ = s.cmd.Wait()
}

func (s *codexAppServer) send(value any) error { return json.NewEncoder(s.input).Encode(value) }

func (s *codexAppServer) next(ctx context.Context) (codexRPCMessage, error) {
	select {
	case <-ctx.Done():
		return codexRPCMessage{}, ctx.Err()
	case message, ok := <-s.messages:
		if !ok {
			if s.readErr != nil {
				return message, s.readErr
			}
			return message, fmt.Errorf("Codex App Server disconnected before completing the request")
		}
		return message, nil
	}
}

// This integration never grants extra local permissions on behalf of the user.
// Remote operations continue through aissh's proposal/approval channel.
func (s *codexAppServer) rejectRequest(message codexRPCMessage) error {
	switch message.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		return s.send(map[string]any{"id": message.ID, "result": map[string]any{"decision": "decline"}})
	default:
		return s.send(map[string]any{"id": message.ID, "error": map[string]any{"code": -32601, "message": "Use the aissh command proposal or ask_user response"}})
	}
}

func (s *codexAppServer) call(ctx context.Context, method string, params any, notify func(codexRPCMessage)) (json.RawMessage, error) {
	stop := context.AfterFunc(ctx, func() { s.stop(); _ = s.input.Close(); _ = s.output.Close() })
	defer stop()
	s.nextID++
	id := fmt.Sprint(s.nextID)
	if err := s.send(map[string]any{"id": s.nextID, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		message, err := s.next(ctx)
		if err != nil {
			return nil, err
		}
		if message.Method != "" {
			if len(message.ID) > 0 {
				if err := s.rejectRequest(message); err != nil {
					return nil, err
				}
			} else if notify != nil {
				notify(message)
			}
			continue
		}
		if string(message.ID) != id {
			continue
		}
		if message.Error != nil {
			return nil, fmt.Errorf("Codex %s: %s", method, message.Error.Message)
		}
		return message.Result, nil
	}
}

func (p *CodexProvider) invokeStreaming(ctx context.Context, prompt, workspace string, writable bool, threadID string, callbacks Callbacks) (result codexResponse, returnedID string, err error) {
	// The terminal client keeps its original exec/resume protocol.
	if !p.Streaming {
		return p.invoke(ctx, prompt, workspace, writable, threadID)
	}
	p.streamMu.Lock()
	defer p.streamMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, p.invocationTimeout())
	defer cancel()
	defer func() {
		if err != nil && p.server != nil {
			p.server.close()
			p.server = nil
		}
	}()
	if p.server == nil {
		start := p.startServer
		if start == nil {
			start = startCodexAppServer
		}
		p.server, err = start(p.Path)
		if err != nil {
			return result, "", err
		}
		_, err = p.server.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "aissh", "title": "WRD AI SSH", "version": "0.1.0"}}, nil)
		if err != nil {
			return result, "", err
		}
		if err = p.server.send(map[string]any{"method": "initialized"}); err != nil {
			return result, "", err
		}
	}
	sandbox := "read-only"
	policy := map[string]any{"type": "readOnly", "networkAccess": false}
	if workspace != "" && writable {
		sandbox = "workspace-write"
		policy = map[string]any{"type": "workspaceWrite", "writableRoots": []string{workspace}, "networkAccess": false, "excludeSlashTmp": true, "excludeTmpdirEnvVar": true}
	}
	params := map[string]any{"cwd": codexWorkingDirectory(workspace), "sandbox": sandbox, "approvalPolicy": "never"}
	if p.Model != "" {
		params["model"] = p.Model
	}
	method := "thread/start"
	if threadID != "" {
		method = "thread/resume"
		params["threadId"] = threadID
	}
	raw, err := p.server.call(ctx, method, params, nil)
	if err != nil {
		if threadID != "" && (strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "no rollout found")) {
			err = fmt.Errorf("session not found: %w", err)
		}
		return result, "", err
	}
	var response struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = json.Unmarshal(raw, &response); err != nil {
		return result, "", err
	}
	returnedID = response.Thread.ID
	if returnedID == "" {
		return result, "", fmt.Errorf("Codex App Server returned no thread ID")
	}
	if threadID == "" {
		p.mu.Lock()
		p.owned[returnedID] = struct{}{}
		p.mu.Unlock()
	}
	stream := newCodexTurnStream(returnedID, callbacks)
	_, err = p.server.call(ctx, "turn/start", map[string]any{
		"threadId": returnedID, "input": []any{map[string]string{"type": "text", "text": prompt}},
		"cwd": codexWorkingDirectory(workspace), "approvalPolicy": "never", "sandboxPolicy": policy,
		"outputSchema": json.RawMessage(codexResponseSchema),
	}, stream.notify)
	if err != nil {
		return result, returnedID, err
	}
	for !stream.done {
		var message codexRPCMessage
		message, err = p.server.next(ctx)
		if err != nil {
			return result, returnedID, err
		}
		if len(message.ID) > 0 && message.Method != "" {
			err = p.server.rejectRequest(message)
			if err != nil {
				return result, returnedID, err
			}
			continue
		}
		stream.notify(message)
	}
	if stream.err != nil {
		return result, returnedID, stream.err
	}
	if err = json.Unmarshal([]byte(stripJSONFence(stream.final)), &result); err != nil {
		return result, returnedID, fmt.Errorf("无法解析 Codex 的结构化结果：%w", err)
	}
	return result, returnedID, nil
}

type codexTurnStream struct {
	threadID   string
	callbacks  Callbacks
	text       map[string]*messageStream
	activities map[string]Activity
	final      string
	done       bool
	err        error
}

func newCodexTurnStream(threadID string, callbacks Callbacks) *codexTurnStream {
	return &codexTurnStream{threadID: threadID, callbacks: callbacks, text: map[string]*messageStream{}, activities: map[string]Activity{}}
}

func (s *codexTurnStream) notify(message codexRPCMessage) {
	var p struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		ItemID   string `json:"itemId"`
		Delta    string `json:"delta"`
		Item     struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Text     string `json:"text"`
			Phase    string `json:"phase"`
			Command  string `json:"command"`
			Tool     string `json:"tool"`
			Status   string `json:"status"`
			Output   string `json:"aggregatedOutput"`
			ExitCode *int   `json:"exitCode"`
			Changes  []struct {
				Path string `json:"path"`
			} `json:"changes"`
		} `json:"item"`
		Turn struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal(message.Params, &p) != nil || p.ThreadID != s.threadID {
		return
	}
	p.Item.ID = p.TurnID + ":" + p.Item.ID
	p.ItemID = p.TurnID + ":" + p.ItemID
	switch message.Method {
	case "item/agentMessage/delta":
		if s.text[p.ItemID] == nil {
			s.text[p.ItemID] = &messageStream{emit: s.callbacks.OnAnswer}
		}
		s.text[p.ItemID].Write(p.Delta)
	case "item/started", "item/completed":
		item := p.Item
		if item.Type == "agentMessage" {
			if message.Method == "item/completed" && item.Phase != "commentary" {
				s.final = item.Text
				if s.text[item.ID] == nil {
					s.text[item.ID] = &messageStream{emit: s.callbacks.OnAnswer}
					s.text[item.ID].Write(stripJSONFence(item.Text))
				}
			}
			return
		}
		a := Activity{ID: item.ID, Kind: item.Type, Status: "running"}
		switch item.Type {
		case "commandExecution":
			a.Title, a.Detail = item.Command, item.Output
		case "fileChange":
			a.Title = "修改文件"
			for _, change := range item.Changes {
				a.Detail += change.Path + "\n"
			}
		case "mcpToolCall", "dynamicToolCall":
			a.Title = item.Tool
		case "contextCompaction":
			a.Title = "整理对话上下文"
		default:
			return
		}
		if message.Method == "item/completed" {
			a.Status = "completed"
		}
		if item.Status == "failed" || item.Status == "declined" || (item.ExitCode != nil && *item.ExitCode != 0) {
			a.Status = "failed"
		}
		s.activities[a.ID] = a
		s.emitActivity(a)
	case "item/commandExecution/outputDelta":
		a, ok := s.activities[p.ItemID]
		if ok {
			a.Detail = tailText(a.Detail+p.Delta, 16000)
			s.activities[a.ID] = a
			s.emitActivity(a)
		}
	case "turn/completed":
		s.done = true
		if p.Turn.Status != "completed" {
			detail := p.Turn.Status
			if p.Turn.Error != nil {
				detail = p.Turn.Error.Message
			}
			s.err = fmt.Errorf("Codex: %s", detail)
		}
	}
}

func (s *codexTurnStream) emitActivity(a Activity) {
	if s.callbacks.OnActivity != nil {
		s.callbacks.OnActivity(a)
	}
}

func tailText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[len(runes)-limit:])
	}
	return value
}
