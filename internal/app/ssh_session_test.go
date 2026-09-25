package app

import (
	"errors"
	"strings"
	"testing"
)

func TestSSHSessionErrorKeepsDiagnosticAndCause(t *testing.T) {
	for _, tc := range []struct {
		language string
		want     string
	}{
		{"zh", "SSH 连接失败或异常结束（server）"},
		{"en", "SSH connection failed or ended unexpectedly (server)"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			a, err := New(Config{SSHArgs: []string{"server"}, Language: tc.language})
			if err != nil {
				t.Fatal(err)
			}
			defer a.cancel()
			cause := errors.New("exit status 255")
			a.terminalErr = cause
			a.outputTail.Append("\x1b[31mssh: connect to host server port 2222: Connection refused\x1b[0m\r\n\a")
			err = a.sshSessionError()
			if !errors.Is(err, cause) {
				t.Fatalf("lost process exit error: %v", err)
			}
			want := tc.want + ": exit status 255\nssh: connect to host server port 2222: Connection refused"
			if err.Error() != want {
				t.Fatalf("diagnostic=%q, want %q", err, want)
			}
			a.outputTail.value = ""
			if err := a.sshSessionError(); err == nil || !strings.Contains(err.Error(), "exit status 255") {
				t.Fatalf("silent failure lost its exit status: %v", err)
			}
		})
	}
}

func TestSSHSessionNormalExitIgnoresEarlierErrorOutput(t *testing.T) {
	a, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.cancel()
	a.outputTail.Append("Permission denied\r\n")
	a.handleEvent(sshExitEvent{})
	if err := a.sshSessionError(); err != nil {
		t.Fatalf("normal logout reported an error: %v", err)
	}
}
