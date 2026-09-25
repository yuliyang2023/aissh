//go:build !windows

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestSSHExitDiagnosticSurvivesTerminalRestore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   string
		failed bool
	}{
		{"refused", "printf 'ssh: connect to host server port 2222: Connection refused\\n' >&2\nexit 255\n", "port 2222: Connection refused", true},
		{"authentication", "printf 'server: Permission denied (publickey).\\n' >&2\nexit 255\n", "Permission denied (publickey)", true},
		{"silent failure", "exit 255\n", "exit status 255", true},
		{"normal logout", "printf 'Connection to server closed.\\n'\nexit 0\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := filepath.Join(t.TempDir(), "ssh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\n"+tc.script), 0700); err != nil {
				t.Fatal(err)
			}
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSSHExitCLIHelper$")
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, "AISSH_") {
					cmd.Env = append(cmd.Env, variable)
				}
			}
			cmd.Env = append(cmd.Env, "AISSH_TEST_SSH_EXIT="+script, "AISSH_CONFIG_DIR="+t.TempDir())
			terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
			if err != nil {
				t.Fatal(err)
			}
			defer terminal.Close()
			output, readErr := io.ReadAll(terminal)
			err = cmd.Wait()
			if ctx.Err() != nil {
				t.Fatalf("SSH exit hung: %v", ctx.Err())
			}
			if readErr != nil && !errors.Is(readErr, syscall.EIO) {
				t.Fatal(readErr)
			}
			if (err != nil) != tc.failed {
				t.Fatalf("exit error=%v, want failure=%v; output=%q", err, tc.failed, output)
			}
			const restore = "\x1b[?1049l"
			index := strings.LastIndex(string(output), restore)
			if index < 0 {
				t.Fatalf("terminal was not restored: %q", output)
			}
			visible := string(output[index+len(restore):])
			if tc.failed {
				if !strings.Contains(visible, "aissh: SSH") || !strings.Contains(visible, tc.want) {
					t.Fatalf("diagnostic missing after terminal restore: %q", visible)
				}
			} else if strings.TrimSpace(visible) != "" {
				t.Fatalf("normal logout reported an error: %q", visible)
			}
		})
	}
}

func TestSSHExitCLIHelper(t *testing.T) {
	script := os.Getenv("AISSH_TEST_SSH_EXIT")
	if script == "" {
		return
	}
	os.Args = []string{"aissh", "--agent", "codex", "--no-history", "--no-workspace", "--lang", "en", "--ssh-path", script, "-p", "2222", "server"}
	main()
	os.Exit(0)
}
