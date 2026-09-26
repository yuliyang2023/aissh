package app

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestSensitiveBracketedPasteForwardsPayloadWithoutWrappers(t *testing.T) {
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
	application.sensitive = true
	application.atPrompt = true
	application.lineInput = []byte("stale-secret")
	application.lineAnchored = true
	application.lineReliable = false
	application.aiLineCapture = true
	application.lastCommand = "previous-command"

	application.handleTerminalInput([]byte("\x1b[200~password\x1b[20"))
	application.handleTerminalInput([]byte("1~\r"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(written), "password\r"; got != want {
		t.Fatalf("sensitive paste=%q, want %q", got, want)
	}
	if bytes.Contains(written, terminalPasteStart) || bytes.Contains(written, terminalPasteEnd) {
		t.Fatalf("bracketed paste wrappers leaked into password input: %q", written)
	}
	if len(application.lineInput) != 0 || application.lineAnchored || !application.lineReliable || application.aiLineCapture {
		t.Fatalf("sensitive input state was retained: input=%q anchored=%v reliable=%v capture=%v",
			application.lineInput, application.lineAnchored, application.lineReliable, application.aiLineCapture)
	}
	if application.lastCommand != "previous-command" {
		t.Fatalf("password replaced last command: %q", application.lastCommand)
	}
}

func TestSensitiveTypedInputPassesThroughWithoutCommandTracking(t *testing.T) {
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
	application.sensitive = true
	application.atPrompt = true
	application.lastCommand = "previous-command"

	application.handleInput([]byte("plain-password\r"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(written), "plain-password\r"; got != want {
		t.Fatalf("sensitive typed input=%q, want %q", got, want)
	}
	if len(application.lineInput) != 0 {
		t.Fatalf("typed password was retained as command input: %q", application.lineInput)
	}
	if application.lastCommand != "previous-command" {
		t.Fatalf("typed password replaced last command: %q", application.lastCommand)
	}
}

func TestPasswordPromptTakesFocusFromResumedConversation(t *testing.T) {
	a, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.cancel()
	a.focus = focusAI
	a.handleOutput([]byte("ops@server's password: "))
	if !a.sensitive || a.focus != focusTerminal {
		t.Fatal("password prompt left AI editor focused")
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	a.sshPTY = writer
	a.handleInput([]byte("login-password\r"))
	writer.Close()
	input, err := io.ReadAll(reader)
	if err != nil || string(input) != "login-password\r" || len(a.editor) != 0 || a.agentBusy {
		t.Fatal("password was not routed exclusively to SSH")
	}
}
