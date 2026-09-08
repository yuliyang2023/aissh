//go:build !windows

package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestIsolateApprovedCommandReportsStatusAndProtectsOuterShell(t *testing.T) {
	for _, shell := range []string{"sh", "bash", "zsh"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			continue
		}
		for _, tc := range []struct {
			name, command, output string
			status                int
		}{
			{"multiline", "printf 'first\\n'\nprintf 'second\\n'", "first\nsecond\n", 0},
			{"exit", "printf 'missing\\n'\nexit 7", "missing\n", 7},
			{"exec", "exec printf 'replaced\\n'", "replaced\n", 0},
			{"syntax_error", "if then", "", 1},
			{"errexit", "set -e; false; printf WRONG", "", 1},
			{"own_trap", "trap 'printf own-trap' 0; exit 9", "own-trap", 9},
			{"redirect_stdout", "exec >/dev/null; printf hidden; exit 3", "", 3},
			{"heredoc", "cat <<'END'\nquoted 'text' and $HOME\nEND", "quoted 'text' and $HOME\n", 0},
		} {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				script := isolateApprovedCommandWithLineEnding(tc.command, "test", "\n")
				cmd := exec.CommandContext(ctx, path)
				cmd.Stdin = strings.NewReader(strings.TrimSuffix(script, "\r") + "\nprintf 'OUTER_ALIVE\\n'\n")
				data, err := cmd.Output()
				if err != nil {
					t.Fatalf("outer shell failed: %v", err)
				}
				state := &executionState{Marker: "test"}
				output, after, status := state.consume(data)
				if status == nil || *status != tc.status || string(output) != tc.output || string(after) != "OUTER_ALIVE\n" {
					t.Fatalf("command=%q output=%q after=%q status=%v raw=%q", tc.command, output, after, status, data)
				}
			})
		}
	}
}

func TestApprovedCommandCompletesInInteractivePTY(t *testing.T) {
	path, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, interrupted := range []bool{false, true} {
		name := "normal"
		if interrupted {
			name = "ctrl_c"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, path, "--noprofile", "--norc", "-i")
			cmd.Env = append(os.Environ(), "PS1=AISSH_TEST_READY> ", "PS2=> ", "TERM=xterm")
			terminal, err := pty.Start(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = terminal.Close()
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}()
			chunks := make(chan []byte, 32)
			go func() {
				defer close(chunks)
				buffer := make([]byte, 4096)
				for {
					n, err := terminal.Read(buffer)
					if n > 0 {
						select {
						case chunks <- append([]byte(nil), buffer[:n]...):
						case <-ctx.Done():
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
			var transcript string
			read := func() []byte {
				t.Helper()
				select {
				case data, ok := <-chunks:
					if !ok {
						t.Fatalf("shell exited early: %q", transcript)
					}
					transcript += string(data)
					return data
				case <-ctx.Done():
					t.Fatalf("PTY timed out: %q", transcript)
					return nil
				}
			}
			for !strings.Contains(transcript, "AISSH_TEST_READY> ") {
				read()
			}
			transcript = ""
			a, requests := executionTestApp(t)
			command := "printf '\\nSTARTED\\n'; sleep 0.05; printf 'DONE\\n'; exit 7"
			wantStatus := "exit_status=7"
			if interrupted {
				command = "printf '\\nSTARTED\\n'; sleep 30"
				wantStatus = "exit_status=130"
			}
			if _, err := terminal.Write([]byte(isolateApprovedCommand(command, "test"))); err != nil {
				t.Fatal(err)
			}
			for !strings.Contains(transcript, "\r\nSTARTED\r\n") {
				a.processSSHOutput(read())
			}
			if interrupted {
				if _, err := terminal.Write([]byte{3}); err != nil {
					t.Fatal(err)
				}
			}
			for a.execution != nil || !strings.Contains(transcript, "AISSH_TEST_READY> ") {
				a.processSSHOutput(read())
			}
			select {
			case request := <-requests:
				if request.Completion != wantStatus || (!interrupted && !strings.HasSuffix(request.Output, "\r\nDONE")) {
					t.Fatalf("result=%#v transcript=%q", request, transcript)
				}
			case <-ctx.Done():
				t.Fatal("PTY result was not submitted")
			}
		})
	}
}
