package main

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParseArgsPreservesSSHOptions(t *testing.T) {
	config, help, version, err := parseArgs([]string{
		"--agent", "http",
		"--agent-url", "http://agent/stream",
		"-p", "2222", "-J", "jump", "root@server",
	})
	if err != nil {
		t.Fatal(err)
	}
	if help || version {
		t.Fatal("unexpected help/version")
	}
	if config.AgentURL != "http://agent/stream" {
		t.Fatalf("agent URL = %q", config.AgentURL)
	}
	if config.AgentProvider != "http" {
		t.Fatalf("agent provider = %q", config.AgentProvider)
	}
	want := []string{"-p", "2222", "-J", "jump", "root@server"}
	if !reflect.DeepEqual(config.SSHArgs, want) {
		t.Fatalf("ssh args = %#v", config.SSHArgs)
	}
}

func TestParseArgsDefaultsToLocalCodex(t *testing.T) {
	t.Setenv("AISSH_AGENT_PROVIDER", "")
	t.Setenv("AISSH_AGENT", "")
	t.Setenv("AISSH_AGENT_URL", "")
	config, help, version, err := parseArgs([]string{"server"})
	if err != nil || help || version {
		t.Fatalf("help=%v version=%v err=%v", help, version, err)
	}
	if config.AgentProvider != "codex" || config.CodexPath != "codex" {
		t.Fatalf("provider=%q codex=%q", config.AgentProvider, config.CodexPath)
	}
}

func TestParseArgsSupportsClaudeProvider(t *testing.T) {
	t.Setenv("AISSH_CLAUDE_PATH", "")
	t.Setenv("AISSH_CLAUDE_MODEL", "")
	config, help, version, err := parseArgs([]string{
		"--agent", "claude",
		"--claude-path", "/opt/bin/claude",
		"--claude-model=opus",
		"server",
	})
	if err != nil || help || version {
		t.Fatalf("help=%v version=%v err=%v", help, version, err)
	}
	if config.AgentProvider != "claude" || config.ClaudePath != "/opt/bin/claude" || config.ClaudeModel != "opus" {
		t.Fatalf("provider=%q path=%q model=%q", config.AgentProvider, config.ClaudePath, config.ClaudeModel)
	}
	if !reflect.DeepEqual(config.SSHArgs, []string{"server"}) {
		t.Fatalf("ssh args = %#v", config.SSHArgs)
	}
}

func TestParseArgsSupportsLanguage(t *testing.T) {
	config, help, version, err := parseArgs([]string{"--lang", "en-US", "server"})
	if err != nil || help || version {
		t.Fatalf("help=%v version=%v err=%v", help, version, err)
	}
	if config.Language != "en" {
		t.Fatalf("language=%q", config.Language)
	}
}

func TestParseArgsSupportsHistoryConfiguration(t *testing.T) {
	t.Setenv("AISSH_HISTORY", "")
	t.Setenv("AISSH_HISTORY_DIR", "")
	config, help, version, err := parseArgs([]string{"--history-dir", "/tmp/aissh-history-test", "server"})
	if err != nil || help || version || config.HistoryDir != "/tmp/aissh-history-test" {
		t.Fatalf("history=%q help=%v version=%v err=%v", config.HistoryDir, help, version, err)
	}
	config, _, _, err = parseArgs([]string{"--no-history", "server"})
	if err != nil || config.HistoryDir != "" {
		t.Fatalf("disabled history=%q err=%v", config.HistoryDir, err)
	}
}

func TestParseArgsSupportsLocalWorkspace(t *testing.T) {
	config, help, version, err := parseArgs([]string{"--workspace", "/tmp/project", "server"})
	if err != nil || help || version || config.Workspace != "/tmp/project" {
		t.Fatalf("workspace=%q help=%v version=%v err=%v", config.Workspace, help, version, err)
	}
}

func TestParseArgsDefaultsWorkspaceToCurrentDirectory(t *testing.T) {
	t.Setenv("AISSH_WORKSPACE", "")
	config, help, version, err := parseArgs([]string{"server"})
	current, currentErr := os.Getwd()
	if err != nil || currentErr != nil || help || version || config.Workspace != current {
		t.Fatalf("workspace=%q current=%q help=%v version=%v err=%v currentErr=%v", config.Workspace, current, help, version, err, currentErr)
	}
	config, _, _, err = parseArgs([]string{"--no-workspace", "server"})
	if err != nil || config.Workspace != "" {
		t.Fatalf("disabled workspace=%q err=%v", config.Workspace, err)
	}
}

func TestParseArgsSupportsDirectModel(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-secret")
	config, help, version, err := parseArgs([]string{
		"--agent", "deepseek", "--model-url", "https://model.example/v1", "--model-name", "deepseek-test", "server",
	})
	if err != nil || help || version {
		t.Fatalf("help=%v version=%v err=%v", help, version, err)
	}
	if config.AgentProvider != "deepseek" || config.ModelURL != "https://model.example/v1" || config.ModelName != "deepseek-test" || config.ModelAPIKey != "deepseek-secret" {
		t.Fatalf("config=%#v", config)
	}
}

func TestParseArgsSupportsScrollbackLines(t *testing.T) {
	t.Setenv("AISSH_SCROLLBACK_LINES", "")
	config, help, version, err := parseArgs([]string{"--scrollback-lines", "40000", "server"})
	if err != nil || help || version || config.ScrollbackLines != 40000 {
		t.Fatalf("scrollback=%d help=%v version=%v err=%v", config.ScrollbackLines, help, version, err)
	}
	if _, _, _, err := parseArgs([]string{"--scrollback-lines", "0", "server"}); err == nil {
		t.Fatal("invalid scrollback line count was accepted")
	}
}

func TestParseArgsSupportsAITimeout(t *testing.T) {
	t.Setenv("AISSH_AI_TIMEOUT", "")
	config, help, version, err := parseArgs([]string{"--ai-timeout", "10m", "server"})
	if err != nil || help || version || config.AITimeout != 10*time.Minute {
		t.Fatalf("timeout=%s help=%v version=%v err=%v", config.AITimeout, help, version, err)
	}
	config, _, _, err = parseArgs([]string{"--ai-timeout=180", "server"})
	if err != nil || config.AITimeout != 3*time.Minute {
		t.Fatalf("numeric timeout=%s err=%v", config.AITimeout, err)
	}
	config, _, _, err = parseArgs([]string{"--ai-timeout", "1h", "server"})
	if err != nil || config.AITimeout != time.Hour {
		t.Fatalf("hour timeout=%s err=%v", config.AITimeout, err)
	}
	if _, _, _, err := parseArgs([]string{"--ai-timeout", "0", "server"}); err == nil {
		t.Fatal("invalid AI timeout was accepted")
	}
}

func TestParseArgsSupportsAIKeepalive(t *testing.T) {
	t.Setenv("AISSH_AI_KEEPALIVE", "")
	config, help, version, err := parseArgs([]string{"--ai-keepalive", "45s", "server"})
	if err != nil || help || version || config.AIKeepalive != 45*time.Second || config.DisableAIKeepalive {
		t.Fatalf("keepalive=%s disabled=%v help=%v version=%v err=%v", config.AIKeepalive, config.DisableAIKeepalive, help, version, err)
	}
	config, _, _, err = parseArgs([]string{"--ai-keepalive=off", "server"})
	if err != nil || config.AIKeepalive != 0 || !config.DisableAIKeepalive {
		t.Fatalf("disabled keepalive=%s disabled=%v err=%v", config.AIKeepalive, config.DisableAIKeepalive, err)
	}
	config, _, _, err = parseArgs([]string{"--no-ai-keepalive", "server"})
	if err != nil || config.AIKeepalive != 0 || !config.DisableAIKeepalive {
		t.Fatalf("flag keepalive=%s disabled=%v err=%v", config.AIKeepalive, config.DisableAIKeepalive, err)
	}
	if _, _, _, err := parseArgs([]string{"--ai-keepalive", "later", "server"}); err == nil {
		t.Fatal("invalid AI keepalive was accepted")
	}
}

func TestParseArgsSupportsAIContextChars(t *testing.T) {
	t.Setenv("AISSH_AI_CONTEXT_CHARS", "")
	config, help, version, err := parseArgs([]string{"--ai-context-chars", "300000", "server"})
	if err != nil || help || version || config.AIContextChars != 300000 {
		t.Fatalf("context chars=%d help=%v version=%v err=%v", config.AIContextChars, help, version, err)
	}
	config, _, _, err = parseArgs([]string{"--ai-context-chars=240000", "server"})
	if err != nil || config.AIContextChars != 240000 {
		t.Fatalf("inline context chars=%d err=%v", config.AIContextChars, err)
	}
	if _, _, _, err := parseArgs([]string{"--ai-context-chars", "0", "server"}); err == nil {
		t.Fatal("invalid AI context character count was accepted")
	}
}
