package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestLocalWorkspaceIsResolvedAndShownInConversation(t *testing.T) {
	workspace := t.TempDir()
	application, err := New(Config{SSHArgs: []string{"server"}, Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if application.config.Workspace != resolved {
		t.Fatalf("workspace=%q want=%q", application.config.Workspace, resolved)
	}
	provider, ok := application.agent.(*agent.CodexProvider)
	if !ok {
		t.Fatalf("provider=%T", application.agent)
	}
	if provider.Workspace != resolved {
		t.Fatalf("provider workspace=%q", provider.Workspace)
	}
	if conversation := strings.Join(application.aiLines(120), "\n"); !strings.Contains(conversation, "本机代码工作区（只读）") {
		t.Fatalf("conversation=%q", conversation)
	}
}

func TestLocalWorkspaceMustBeDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{SSHArgs: []string{"server"}, Workspace: path}); err == nil || !strings.Contains(err.Error(), "代码工作区") {
		t.Fatalf("workspace error=%v", err)
	}
}

func TestHTTPAgentIgnoresLocalWorkspace(t *testing.T) {
	application, err := New(Config{
		SSHArgs: []string{"server"}, AgentProvider: "http", AgentURL: "http://agent/stream", Workspace: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if application.config.Workspace != "" {
		t.Fatalf("HTTP workspace=%q", application.config.Workspace)
	}
}

func TestDeepSeekUsesOfficialDefaultsAndDoesNotExposeWorkspace(t *testing.T) {
	application, err := New(Config{
		SSHArgs: []string{"server"}, AgentProvider: "deepseek", ModelAPIKey: "secret", Workspace: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := application.agent.(*agent.DirectModelProvider)
	if !ok {
		t.Fatalf("provider=%T", application.agent)
	}
	if provider.BaseURL != "https://api.deepseek.com" || provider.Model != "deepseek-v4-pro" || application.config.Workspace != "" {
		t.Fatalf("provider=%#v workspace=%q", provider, application.config.Workspace)
	}
	conversation := strings.Join(application.aiLines(120), "\n")
	if !strings.Contains(conversation, "直连模型模式") || strings.Contains(conversation, "代码工作区（只读）") {
		t.Fatalf("conversation=%q", conversation)
	}
}

func TestAITimeoutAppliesToEveryProvider(t *testing.T) {
	timeout := 9 * time.Minute
	codexApp, err := New(Config{SSHArgs: []string{"server"}, AITimeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	if provider := codexApp.agent.(*agent.CodexProvider); provider.Timeout != timeout {
		t.Fatalf("Codex timeout=%s", provider.Timeout)
	}

	modelApp, err := New(Config{
		SSHArgs: []string{"server"}, AgentProvider: "deepseek", ModelAPIKey: "secret", AITimeout: timeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider := modelApp.agent.(*agent.DirectModelProvider); provider.HTTP.Timeout != timeout {
		t.Fatalf("model timeout=%s", provider.HTTP.Timeout)
	}

	httpApp, err := New(Config{
		SSHArgs: []string{"server"}, AgentProvider: "http", AgentURL: "http://agent/stream", AITimeout: timeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider := httpApp.agent.(*agent.Client); provider.HTTP.Timeout != timeout {
		t.Fatalf("HTTP Agent timeout=%s", provider.HTTP.Timeout)
	}
}
