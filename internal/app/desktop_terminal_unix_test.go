//go:build !windows

package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// Exercise Run -> child SSH PTY -> desktop output, including startup/teardown.
// Replaying these bytes in the browser catches a CLI alternate-screen wrapper
// that a frontend-only mock of ordinary SSH output would miss.
func TestDesktopTerminalPTYScrollback(t *testing.T) {
	script := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
printf 'PTY_SIZE='
stty size
i=0
while [ "$i" -lt 400 ]; do
  printf 'real SSH line %s\n' "$i"
  i=$((i + 1))
done
printf 'SSH_OUTPUT_END\n'
`), 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestDesktopTerminalPTYHelper$")
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "AISSH_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "AISSH_TEST_DESKTOP_SSH="+script)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	output, readErr := io.ReadAll(terminal)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("desktop terminal exited: %v; output=%q", err, output)
	}
	if readErr != nil && !errors.Is(readErr, syscall.EIO) {
		t.Fatal(readErr)
	}
	if !bytes.Contains(output, []byte("PTY_SIZE=24 100\r\n")) {
		t.Fatalf("SSH size differs from the desktop viewport: %q", output)
	}
	for i := 0; i < 400; i++ {
		line := []byte(fmt.Sprintf("real SSH line %d\r\n", i))
		if bytes.Count(output, line) != 1 {
			t.Fatalf("SSH line %d was lost or repainted", i)
		}
	}
	for _, forbidden := range []string{"\x1b[?1049h", "\x1b[2J", "AI 对话与监控由客户端显示", " aissh "} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Fatalf("CLI screen control %q prevents desktop scrollback", forbidden)
		}
	}
	if target := os.Getenv("AISSH_TEST_DESKTOP_TRANSCRIPT"); target != "" {
		if err := os.WriteFile(target, output, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDesktopTerminalPTYHelper(t *testing.T) {
	ssh := os.Getenv("AISSH_TEST_DESKTOP_SSH")
	if ssh == "" {
		return
	}
	a, err := New(Config{DesktopUI: true, SSHPath: ssh, SSHArgs: []string{"server"}})
	if err == nil {
		err = a.Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
