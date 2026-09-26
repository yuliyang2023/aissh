package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestOnlyDesktopEnablesProviderStreaming(t *testing.T) {
	for _, desktop := range []bool{false, true} {
		for _, mode := range []string{"codex", "claude", "deepseek", "model"} {
			provider, err := newAgentProvider(Config{AgentProvider: mode, DesktopUI: desktop})
			if err != nil {
				t.Fatal(err)
			}
			var enabled bool
			switch p := provider.(type) {
			case *agent.CodexProvider:
				enabled = p.Streaming
			case *agent.ClaudeProvider:
				enabled = p.Streaming
			case *agent.DirectModelProvider:
				enabled = p.Streaming
			}
			if enabled != desktop {
				t.Fatalf("mode=%s desktop=%v streaming=%v", mode, desktop, enabled)
			}
		}
	}
}

func TestDesktopStreamsAcrossActivityUpdatesWithoutDuplicateReply(t *testing.T) {
	a := desktopTestApp(t)
	a.chat = nil
	a.appendAssistantDelta("实时")
	a.updateAgentActivity(agent.Activity{ID: "tool", Title: "cat README.md", Status: "running"})
	a.appendAssistantDelta("回复")
	a.updateAgentActivity(agent.Activity{ID: "tool", Title: "cat README.md", Status: "completed", Detail: "super-secret"})
	if len(a.chat) != 2 || a.chat[0].Content != "实时回复" {
		t.Fatalf("chat=%#v", a.chat)
	}
	a.finishAgentTurn(agent.Turn{Message: "实时回复。"}, nil)
	if len(a.chat) != 2 || a.chat[0].Role != "assistant" || a.chat[0].Content != "实时回复。" {
		t.Fatalf("chat=%#v", a.chat)
	}
	state := a.desktopState()
	if strings.Contains(state.Messages[1].Activity.Detail, "super-secret") {
		t.Fatal("activity exposed secret")
	}
	a.chat[1].Activity.Detail = "changed"
	if state.Messages[1].Activity.Detail == "changed" {
		t.Fatal("mutable activity snapshot")
	}
}

func TestDesktopStreamFailureRetainsTextAndStopsActivity(t *testing.T) {
	a := desktopTestApp(t)
	a.chat = nil
	a.appendAssistantDelta("保留已收到的文字")
	a.updateAgentActivity(agent.Activity{ID: "tool", Title: "读取文件", Status: "running"})
	a.finishAgentTurn(agent.Turn{}, errors.New("connection interrupted"))
	if len(a.chat) != 3 || a.chat[0].Content != "保留已收到的文字" || a.chat[0].Role != "assistant" || a.chat[1].Activity.Status != "unknown" || a.chat[2].Role != "error" {
		t.Fatalf("chat=%#v", a.chat)
	}
}

func TestDesktopCommandKeepsOneRowAndNeverGuessesSuccess(t *testing.T) {
	a := desktopTestApp(t)
	a.chat = []chatMessage{{Role: "proposal", Time: time.Now(), Proposal: &agent.Proposal{ID: "p", Command: "echo super-secret"}}}
	a.updateSSHActivity("p", "running", "")
	a.execution = &executionState{ProposalID: "p", Output: newContextBuffer(1000)}
	a.execution.Output.Append("super-secret output")
	state := a.desktopState()
	if len(state.Messages) != 1 || state.Messages[0].Proposal == nil || state.Messages[0].Activity.Status != "running" || strings.Contains(state.Messages[0].Activity.Detail, "super-secret") {
		t.Fatal("invalid command snapshot")
	}
	a.finishSSHActivity(a.execution)
	if a.chat[0].Activity.Status != "unknown" {
		t.Fatal("unknown exit was reported as success")
	}
	code := 1
	a.execution.ExitCode = &code
	a.finishSSHActivity(a.execution)
	if a.chat[0].Activity.Status != "failed" || len(a.chat) != 1 {
		t.Fatal("completion added a row or lost failure")
	}
}

func TestTerminalDoesNotAddDesktopActivities(t *testing.T) {
	a := &App{}
	a.updateAgentActivity(agent.Activity{ID: "tool", Title: "read", Status: "running"})
	if len(a.chat) != 0 {
		t.Fatal("desktop tool progress leaked to terminal")
	}
}
