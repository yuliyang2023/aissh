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

func TestWritableLocalWorkspaceIsPropagatedAndShown(t *testing.T) {
	workspace := t.TempDir()
	application, err := New(Config{
		SSHArgs: []string{"server"}, Workspace: workspace, WorkspaceWritable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, ok := application.agent.(*agent.CodexProvider)
	if !ok {
		t.Fatalf("provider=%T", application.agent)
	}
	if !provider.WorkspaceWritable {
		t.Fatal("provider did not receive writable workspace mode")
	}
	conversation := strings.Join(application.aiLines(120), "\n")
	if !strings.Contains(conversation, "本机代码工作区（可修改）") {
		t.Fatalf("conversation=%q", conversation)
	}
	message, err := application.applyLocalWorkspaceCommand("")
	if err != nil || !strings.Contains(message, "可修改") {
		t.Fatalf("message=%q err=%v", message, err)
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

func TestLocalWorkspaceCanBeChangedAfterStartup(t *testing.T) {
	initial := t.TempDir()
	next := t.TempDir()
	application, err := New(Config{SSHArgs: []string{"server"}, Workspace: initial})
	if err != nil {
		t.Fatal(err)
	}
	message, err := application.applyLocalWorkspaceCommand(next)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(next)
	if err != nil {
		t.Fatal(err)
	}
	provider := application.agent.(*agent.CodexProvider)
	if application.config.Workspace != resolved || provider.LocalWorkspace() != resolved {
		t.Fatalf("config=%q provider=%q want=%q", application.config.Workspace, provider.LocalWorkspace(), resolved)
	}
	if !strings.Contains(message, resolved) || !strings.Contains(message, "后续") {
		t.Fatalf("message=%q", message)
	}
}

func TestWorkspaceCommandWorksInAIConversationAndCanDisableWorkspace(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	application.focus = focusAI
	application.handleAIInput([]byte("/workspace off\r"))
	provider := application.agent.(*agent.CodexProvider)
	if application.config.Workspace != "" || provider.LocalWorkspace() != "" || application.agentBusy {
		t.Fatalf("config=%q provider=%q busy=%v", application.config.Workspace, provider.LocalWorkspace(), application.agentBusy)
	}
	if len(application.editor) != 0 || !strings.Contains(application.chat[len(application.chat)-1].Content, "已关闭") {
		t.Fatalf("editor=%q chat=%#v", string(application.editor), application.chat)
	}
}

func TestWorkspaceCommandAtSSHPromptIsHandledLocally(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	application.sshPTY = writer
	application.atPrompt = true
	application.lineInput = []byte("/workspace off")
	application.submitTerminalLine('\r')
	forwarded := make([]byte, 1)
	if count, err := reader.Read(forwarded); err != nil || count != 1 || forwarded[0] != 0x15 {
		t.Fatalf("forwarded count=%d data=%v err=%v", count, forwarded[:count], err)
	}
	if application.inlineAI == nil || application.inlineAI.Prompt != "/workspace off" || application.inlineAI.Error != "" {
		t.Fatalf("inline=%#v", application.inlineAI)
	}
	if application.config.Workspace != "" || !application.atPrompt {
		t.Fatalf("workspace=%q atPrompt=%v", application.config.Workspace, application.atPrompt)
	}
}

func TestWorkspaceCommandIsRejectedForDirectModel(t *testing.T) {
	application, err := New(Config{
		SSHArgs: []string{"server"}, AgentProvider: "deepseek", ModelAPIKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.applyLocalWorkspaceCommand(t.TempDir()); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("error=%v", err)
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
