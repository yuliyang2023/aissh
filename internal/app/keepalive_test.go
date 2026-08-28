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

func TestShellKeepaliveDelayLeavesSafetyMargin(t *testing.T) {
	if got, want := shellKeepaliveDelay(10*time.Minute), 9*time.Minute; got != want {
		t.Fatalf("delay=%s want=%s", got, want)
	}
}

func TestAIKeepaliveTouchesSafeEmptyShellPromptNearTMOUT(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, ShellTMOUT: 10 * time.Second})
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
	application.promptIdleSince = now.Add(-9 * time.Second)
	if !application.maintainAIKeepalive(now) {
		t.Fatal("keepalive was not sent")
	}
	forwarded := make([]byte, len(shellIdleKeepalive))
	if count, err := reader.Read(forwarded); err != nil || count != len(shellIdleKeepalive) || !reflect.DeepEqual(forwarded, shellIdleKeepalive) {
		t.Fatalf("forwarded count=%d data=%v err=%v", count, forwarded[:count], err)
	}
	if application.atPrompt || !application.promptIdleSince.IsZero() {
		t.Fatal("keepalive did not wait for the next prompt")
	}
}

func TestAIKeepaliveUsesExistingPromptIdleTime(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, ShellTMOUT: 10 * time.Minute})
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
	application.promptIdleSince = now.Add(-8 * time.Minute)
	if application.maintainAIKeepalive(now) {
		t.Fatal("keepalive was sent before the TMOUT safety window")
	}
	if !application.maintainAIKeepalive(now.Add(time.Minute)) {
		t.Fatal("keepalive ignored idle time from before AI started")
	}
}

func TestAIKeepaliveDoesNotTouchUnsafeTerminalState(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, ShellTMOUT: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	application.agentBusy = true
	application.atPrompt = true
	application.lineReliable = true
	application.promptIdleSince = time.Now().Add(-time.Minute)
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

func TestShellKeepaliveOnlyRunsWhileAIBusy(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, ShellTMOUT: time.Second})
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
	application.lineReliable = true
	application.promptIdleSince = time.Now().Add(-time.Minute)
	if application.maintainAIKeepalive(time.Now()) {
		t.Fatal("keepalive was sent while AI was idle")
	}
}

func TestSSHKeepaliveCanBeDisabledWithoutEnablingShellInput(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}, DisableSSHKeepalive: true})
	if err != nil {
		t.Fatal(err)
	}
	if application.config.SSHKeepalive != 0 || sshServerAliveOptions(application.config.SSHKeepalive) != nil || application.config.ShellTMOUT != 0 {
		t.Fatalf("ssh keepalive=%s shell tmout=%s", application.config.SSHKeepalive, application.config.ShellTMOUT)
	}
}
