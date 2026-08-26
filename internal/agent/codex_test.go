package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/i18n"
)

func TestCodexProviderReturnsProposalAndPanel(t *testing.T) {
	provider := testCodexProvider(t, `{"type":"command_proposal","message":"先查看负载。","proposal":{"id":"","command":"uptime","explanation":"检查系统负载","expectedEffect":"输出负载均值","risk":"read_only"},"panelActions":[{"action":"open","panel":{"id":"load","type":"monitor","title":"负载","collector":"system.overview","command":"","intervalMs":3000,"content":"","allowAIRead":true}}]}`)
	var action PanelAction
	turn, err := provider.Stream(context.Background(), Request{
		Event:  "start",
		Prompt: "帮我看看负载",
		Target: &Target{HostLabel: "root@server", Protocol: "ssh"},
	}, Callbacks{OnPanelAction: func(value PanelAction) { action = value }})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Session.ID == "" || turn.Proposal == nil || turn.Proposal.Command != "uptime" {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Proposal.ID == "" || action.Panel.ID != "load" {
		t.Fatalf("proposal=%#v action=%#v", turn.Proposal, action)
	}

	approved, err := provider.Action(context.Background(), Request{
		Event: "approve", SessionID: turn.Session.ID, ProposalID: turn.Proposal.ID,
	})
	if err != nil || approved.Type != "approved" {
		t.Fatalf("approved=%#v err=%v", approved, err)
	}
}

func TestCodexProviderCarriesExecutionResult(t *testing.T) {
	provider := testCodexProvider(t, `{"type":"finished","message":"负载正常。","proposal":null,"panelActions":[]}`)
	provider.mu.Lock()
	provider.sessions["s1"] = &codexSession{
		ID: "s1",
		Proposals: map[string]Proposal{
			"p1": {ID: "p1", Command: "uptime"},
		},
	}
	provider.mu.Unlock()
	turn, err := provider.Stream(context.Background(), Request{
		Event: "result", SessionID: "s1", ProposalID: "p1", Output: "load average: 0.10", Completion: "prompt",
	}, Callbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if turn.Message != "负载正常。" {
		t.Fatalf("turn = %#v", turn)
	}
}

func TestParseCodexJSONL(t *testing.T) {
	response, err := parseCodexJSONL([]byte(
		"{\"type\":\"thread.started\",\"thread_id\":\"t1\"}\n" +
			"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"{\\\"type\\\":\\\"finished\\\",\\\"message\\\":\\\"完成\\\",\\\"proposal\\\":null,\\\"panelActions\\\":[]}\"}}\n",
	))
	if err != nil || response.Message != "完成" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}

func TestCodexAuthErrorIsSanitized(t *testing.T) {
	err := codexRunError(errors.New("exit status 1"), nil, []byte("401 Unauthorized: Incorrect API key provided: secret-value"))
	if strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("error = %q", err)
	}
}

func TestCodexProviderLocalizesStatusAndErrors(t *testing.T) {
	provider := NewCodexProvider("missing-codex", "")
	provider.Language = "en"
	provider.lookPath = func(string) (string, error) { return "", errors.New("missing") }
	if _, err := provider.Stream(context.Background(), Request{Event: "start"}, Callbacks{}); err == nil || !strings.Contains(err.Error(), "Local Codex CLI") {
		t.Fatalf("localized missing CLI error=%v", err)
	}

	provider = testCodexProvider(t, `{"type":"finished","message":"done","proposal":null,"panelActions":[]}`)
	provider.Language = "en"
	var status string
	_, err := provider.Stream(context.Background(), Request{
		Event: "start", Prompt: "check", SystemContext: map[string]string{"ui_language": "en"},
	}, Callbacks{OnStatus: func(value string) { status = value }})
	if err != nil || status != "Calling local Codex" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	localized := codexRunErrorLocalized(errors.New("exit status 1"), nil, []byte("401 Unauthorized"), i18n.New("en"))
	if !strings.Contains(localized.Error(), "authentication failed") {
		t.Fatalf("localized auth error=%q", localized)
	}
}

func TestCodexPromptFollowsQuestionLanguageInsteadOfUILanguage(t *testing.T) {
	provider := testCodexProvider(t, `{"type":"finished","message":"可以，先检查服务状态。","proposal":null,"panelActions":[]}`)
	provider.Language = "en"
	provider.run = func(_ context.Context, _ string, _ []string, prompt string) ([]byte, []byte, error) {
		if !strings.Contains(prompt, "本轮用户使用中文提问") || !strings.Contains(prompt, "response_language=zh-CN") {
			t.Fatalf("prompt did not select Chinese response: %s", prompt)
		}
		response := `{"type":"finished","message":"可以，先检查服务状态。","proposal":null,"panelActions":[]}`
		line := `{"type":"item.completed","item":{"type":"agent_message","text":` + string(mustJSON(response)) + `}}` + "\n"
		return []byte(line), nil, nil
	}
	_, err := provider.Stream(context.Background(), Request{
		Event: "start", Prompt: "帮我检查 nginx", SystemContext: map[string]string{"ui_language": "en"},
	}, Callbacks{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCodexPromptContinuesSelectedLocalHistory(t *testing.T) {
	prompt := buildCodexPrompt(Request{
		Prompt: "继续查",
		SystemContext: map[string]string{
			"conversation_history": "用户：检查 nginx\n\n助手：发现上游超时",
		},
	}, nil, "用户请求：继续查")
	for _, want := range []string{"继续一个本机历史会话", "用户：检查 nginx", "助手：发现上游超时", "不要声称仍连接着旧的运行时 Agent Session"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("history prompt missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "conversation_history=") {
		t.Fatalf("history context leaked into generic environment block: %s", prompt)
	}
}

func TestCodexProviderUsesReadOnlyLocalWorkspace(t *testing.T) {
	workspace := t.TempDir()
	provider := NewCodexProvider("codex", "")
	provider.Workspace = workspace
	provider.lookPath = func(string) (string, error) { return "/usr/local/bin/codex", nil }
	provider.run = func(_ context.Context, _ string, args []string, prompt string) ([]byte, []byte, error) {
		if !containsArgs(args, "-C", workspace) || !containsArgs(args, "--sandbox", "read-only") {
			t.Fatalf("workspace args=%#v", args)
		}
		for _, want := range []string{"local_workspace=" + workspace, "本机工作区严格只读", "command_proposal 永远表示将在目标服务器"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("workspace prompt missing %q: %s", want, prompt)
			}
		}
		response := `{"type":"finished","message":"done","proposal":null,"panelActions":[]}`
		line := `{"type":"item.completed","item":{"type":"agent_message","text":` + string(mustJSON(response)) + `}}` + "\n"
		return []byte(line), nil, nil
	}
	if _, err := provider.Stream(context.Background(), Request{Event: "start", Prompt: "check this deployment"}, Callbacks{}); err != nil {
		t.Fatal(err)
	}
}

func TestCodexProviderWorkspaceCanBeUpdatedBetweenTurns(t *testing.T) {
	provider := NewCodexProvider("codex", "")
	workspace := t.TempDir()
	provider.SetLocalWorkspace(workspace)
	if got := provider.LocalWorkspace(); got != workspace {
		t.Fatalf("workspace=%q want=%q", got, workspace)
	}
	provider.SetLocalWorkspace("")
	if got := provider.LocalWorkspace(); got != "" {
		t.Fatalf("disabled workspace=%q", got)
	}
}

func testCodexProvider(t *testing.T, response string) *CodexProvider {
	t.Helper()
	provider := NewCodexProvider("codex", "")
	provider.lookPath = func(string) (string, error) { return "/usr/local/bin/codex", nil }
	provider.run = func(_ context.Context, _ string, args []string, prompt string) ([]byte, []byte, error) {
		if !strings.Contains(prompt, "不能直接操作目标服务器") {
			t.Fatalf("missing safety instruction: %s", prompt)
		}
		if !strings.Contains(prompt, "不要调用任何本机工具") {
			t.Fatalf("unconfigured workspace unexpectedly enabled local tools: %s", prompt)
		}
		if !containsArgs(args, "--sandbox", "read-only") || !containsArgs(args, "--ephemeral") {
			t.Fatalf("unsafe codex args: %#v", args)
		}
		line := `{"type":"item.completed","item":{"type":"agent_message","text":` + string(mustJSON(response)) + `}}` + "\n"
		return []byte(line), nil, nil
	}
	return provider
}

func mustJSON(value string) []byte {
	result := make([]byte, 0, len(value)+2)
	result = append(result, '"')
	for _, current := range []byte(value) {
		switch current {
		case '\\', '"':
			result = append(result, '\\', current)
		case '\n':
			result = append(result, '\\', 'n')
		default:
			result = append(result, current)
		}
	}
	return append(result, '"')
}

func containsArgs(args []string, want ...string) bool {
	if len(want) == 0 || len(want) > len(args) {
		return false
	}
	for index := 0; index <= len(args)-len(want); index++ {
		match := true
		for offset := range want {
			if args[index+offset] != want[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
