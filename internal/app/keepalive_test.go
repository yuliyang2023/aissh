package app

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestSSHServerAliveOptionsUseConfiguredInterval(t *testing.T) {
	want := []string{"-o", "ServerAliveInterval=31", "-o", "ServerAliveCountMax=3"}
	if got := sshServerAliveOptions(30*time.Second + time.Millisecond); !reflect.DeepEqual(got, want) {
		t.Fatalf("options=%#v want=%#v", got, want)
	}
}

func TestAIKeepaliveTouchesSafeEmptyShellPrompt(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, AIKeepalive: 10 * time.Second})
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
	application.agentBusy = true
	application.atPrompt = true
	application.lineReliable = true
	now := time.Now()
	application.lastAIKeepalive = now.Add(-time.Minute)
	if !application.maintainAIKeepalive(now) {
		t.Fatal("keepalive was not sent")
	}
	forwarded := make([]byte, len(shellIdleKeepalive))
	if count, err := reader.Read(forwarded); err != nil || count != len(shellIdleKeepalive) || !reflect.DeepEqual(forwarded, shellIdleKeepalive) {
		t.Fatalf("forwarded count=%d data=%v err=%v", count, forwarded[:count], err)
	}
	if application.maintainAIKeepalive(now.Add(5 * time.Second)) {
		t.Fatal("keepalive ignored its interval")
	}
}

func TestAIKeepaliveDoesNotTouchUnsafeTerminalState(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, AIKeepalive: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	application.agentBusy = true
	application.atPrompt = true
	application.lineReliable = true
	application.lineInput = []byte("partially typed")
	if application.maintainAIKeepalive(time.Now()) {
		t.Fatal("keepalive modified a non-empty shell line")
	}
	application.lineInput = nil
	application.atPrompt = false
	if application.maintainAIKeepalive(time.Now()) {
		t.Fatal("keepalive was sent outside a shell prompt")
	}
}

func TestAIKeepaliveCanBeDisabled(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, DisableAIKeepalive: true})
	if err != nil {
		t.Fatal(err)
	}
	if application.config.AIKeepalive != 0 || sshServerAliveOptions(application.config.AIKeepalive) != nil {
		t.Fatalf("keepalive=%s", application.config.AIKeepalive)
	}
}
