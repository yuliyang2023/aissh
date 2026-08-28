package app

import (
	"context"
	"io"
	"os"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

type terminalInputTestProvider struct{}

func (terminalInputTestProvider) Name() string     { return "test" }
func (terminalInputTestProvider) Configured() bool { return true }
func (terminalInputTestProvider) Stream(context.Context, agent.Request, agent.Callbacks) (agent.Turn, error) {
	return agent.Turn{}, nil
}
func (terminalInputTestProvider) Action(context.Context, agent.Request) (agent.Turn, error) {
	return agent.Turn{}, nil
}

func TestCtrlCClearsTrackedLineBeforeNextChineseInput(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	application.sshPTY = writer
	application.atPrompt = true

	application.handleTerminalInput([]byte("unfinished"))
	application.handleTerminalInput([]byte{0x03})
	application.handleTerminalInput([]byte("这"))

	if got := string(application.lineInput); got != "这" || !application.aiLineCapture {
		t.Fatalf("next Chinese input was not captured: input=%q capture=%v", got, application.aiLineCapture)
	}
	if application.lineCursor != len(application.lineInput) {
		t.Fatalf("line cursor=%d, want %d", application.lineCursor, len(application.lineInput))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(written), "unfinished\x03"; got != want {
		t.Fatalf("SSH input=%q, want %q", got, want)
	}
}

func TestCapturedChineseArrowEditingDoesNotLeakIntoSSH(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	application.sshPTY = writer
	application.agent = terminalInputTestProvider{}
	application.atPrompt = true

	application.handleTerminalInput([]byte("机器"))
	application.handleTerminalInput([]byte("\x1b[D"))
	application.handleTerminalInput([]byte("好"))
	application.handleTerminalInput([]byte{0x1b})
	application.handleTerminalInput([]byte("[C"))
	application.handleTerminalInput([]byte("吗"))

	if got, want := string(application.lineInput), "机好器吗"; got != want {
		t.Fatalf("edited captured line=%q, want %q", got, want)
	}
	application.handleTerminalInput([]byte{'\r'})
	if application.inlineAI == nil || application.inlineAI.Prompt != "机好器吗" {
		t.Fatalf("inline AI prompt=%#v", application.inlineAI)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 0 {
		t.Fatalf("captured edit leaked into SSH: %q", written)
	}
}
