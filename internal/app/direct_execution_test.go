package app

import (
	"strings"
	"testing"
	"time"
)

func TestDirectExecutionSendsApprovedCommandAndWaitsForFreshPrompt(t *testing.T) {
	for _, alt := range []bool{false, true} {
		a, requests := executionTestApp(t)
		state := a.execution
		a.execution = nil
		prefix := ""
		if alt {
			prefix = "\x1b[?1049h"
		}
		a.processSSHOutput([]byte(prefix + "[deployer@node ~]$ "))
		a.execution = state
		a.atPrompt = false
		terminal := &recoveryTestPTY{}
		a.sshPTY = terminal
		state.Command = "ps -eo pid,%cpu,comm --sort=-%cpu | head -n 11"
		a.sendApprovedCommand(state)
		awaitExecutionWrite(t, a)
		if terminal.String() != state.Command+"\r" {
			t.Fatalf("extra commands sent: %q", terminal.String())
		}
		a.checkExecutionWait(time.Now().Add(time.Second))
		if state.finished() {
			t.Fatal("stale pre-command prompt completed execution")
		}
		a.processSSHOutput([]byte("\x1b[?2004h"))
		a.checkExecutionWait(time.Now().Add(time.Second))
		if state.finished() {
			t.Fatal("terminal mode reply reused the pre-command prompt")
		}
		a.processSSHOutput([]byte("\r\n授权模式:\r\n1>远程授权\r\n输入 Q 退出"))
		a.checkExecutionWait(time.Now().Add(time.Second))
		if state.finished() {
			t.Fatal("approval menu completed execution")
		}
		a.processSSHOutput([]byte("\r\nPID CPU COMMAND\r\n123 99 worker\r\n[deployer@no"))
		a.checkExecutionWait(time.Now().Add(time.Second))
		if state.finished() {
			t.Fatal("partial prompt completed execution")
		}
		a.processSSHOutput([]byte("de ~]$ "))
		a.checkExecutionWait(state.LastOutputAt)
		if state.finished() {
			t.Fatal("prompt completed execution before quiet period")
		}
		a.checkExecutionWait(state.LastOutputAt.Add(executionPromptQuietDelay))
		a.submitExecutionResult()
		select {
		case request := <-requests:
			if request.Completion != "shell_prompt_returned; exit_status=unknown" || !strings.Contains(request.Output, "123 99 worker") {
				t.Fatalf("result=%+v", request)
			}
		case <-time.After(time.Second):
			t.Fatal("result not submitted after prompt returned")
		}
		if !a.atPrompt || a.execution != nil {
			t.Fatal("terminal did not become ready after command")
		}
	}
}

func TestDirectPromptCannotFinishWhileCommandIsStillSending(t *testing.T) {
	a, _ := executionTestApp(t)
	state := a.execution
	state.Direct = true
	state.WriteJob = &sshWriteJob{}
	a.processSSHOutput([]byte("\r\n[root@node ~]# "))
	a.checkExecutionWait(state.LastOutputAt.Add(time.Second))
	if state.finished() {
		t.Fatal("prompt ended command before queued bytes drained")
	}
	state.WriteJob.done.Store(true)
	a.checkExecutionWait(state.LastOutputAt.Add(time.Second))
	if state.Completion != "shell_prompt_returned" || state.ExitCode != nil {
		t.Fatalf("completion=%q exit=%v", state.Completion, state.ExitCode)
	}
}
