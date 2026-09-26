package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

func runClaudeStreaming(ctx context.Context, path string, args []string, dir, prompt string, callbacks Callbacks) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Dir, cmd.Stdin = localProviderEnvironment("claude", path), dir, strings.NewReader(prompt)
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		stdout.Close()
		return nil, nil, err
	}
	result, readErr := readClaudeStream(stdout, callbacks)
	if readErr != nil {
		_ = cmd.Process.Kill()
	}
	runErr := cmd.Wait()
	if runErr != nil {
		return result, stderr.Bytes(), runErr
	}
	return result, stderr.Bytes(), readErr
}

func readClaudeStream(reader io.Reader, cb Callbacks) ([]byte, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	streams := map[int]*messageStream{}
	activities := map[string]Activity{}
	var result []byte
	for scanner.Scan() {
		var event struct {
			Type   string  `json:"type"`
			Parent *string `json:"parent_tool_use_id"`
			Event  struct {
				Type  string `json:"type"`
				Index int    `json:"index"`
				Block struct {
					Type string `json:"type"`
					Name string `json:"name"`
					Text string `json:"text"`
				} `json:"content_block"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			} `json:"event"`
			Message struct {
				Content []struct {
					Type      string          `json:"type"`
					ID        string          `json:"id"`
					Name      string          `json:"name"`
					ToolUseID string          `json:"tool_use_id"`
					IsError   bool            `json:"is_error"`
					Input     json.RawMessage `json:"input"`
					Content   json.RawMessage `json:"content"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return result, err
		}
		if event.Parent != nil {
			continue
		}
		switch event.Type {
		case "result":
			result = append([]byte(nil), scanner.Bytes()...)
		case "stream_event":
			e := event.Event
			if e.Type == "message_start" {
				streams = map[int]*messageStream{}
			}
			if e.Type == "content_block_start" && (e.Block.Type == "text" || (e.Block.Type == "tool_use" && e.Block.Name == "StructuredOutput")) {
				streams[e.Index] = &messageStream{emit: cb.OnAnswer}
				streams[e.Index].Write(e.Block.Text)
			}
			if e.Type == "content_block_delta" && streams[e.Index] != nil {
				if e.Delta.Type == "text_delta" {
					streams[e.Index].Write(e.Delta.Text)
				}
				if e.Delta.Type == "input_json_delta" {
					streams[e.Index].Write(e.Delta.PartialJSON)
				}
			}
		case "assistant", "user":
			for _, block := range event.Message.Content {
				if block.Type == "tool_use" && block.Name != "StructuredOutput" {
					a := Activity{ID: "claude:" + block.ID, Kind: "tool", Title: block.Name, Status: "running", Detail: string(block.Input)}
					activities[block.ID] = a
					if cb.OnActivity != nil {
						cb.OnActivity(a)
					}
				}
				if block.Type == "tool_result" {
					if a, ok := activities[block.ToolUseID]; ok {
						a.Status = "completed"
						if block.IsError {
							a.Status = "failed"
						}
						a.Detail = tailText(a.Detail+"\n"+string(block.Content), 16000)
						if cb.OnActivity != nil {
							cb.OnActivity(a)
						}
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("Claude 连接中断，回复尚未完成")
	}
	return result, nil
}
