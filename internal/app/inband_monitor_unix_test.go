//go:build !windows

package app

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

func TestRepeatedInBandMonitorsKeepInteractiveShellInPlace(t *testing.T) {
	shell, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.cancel()
	a.setViewportSize(140, 30)
	a.virtual = vt10x.New(vt10x.WithSize(a.mainCols, a.contentRows))
	a.monitorViaPrimary = true
	a.panels.ByID("system").Command = "printf 'HOST=server\\n'"
	cmd := exec.CommandContext(ctx, shell, "--noprofile", "--norc", "-i")
	cmd.Env = append(os.Environ(), "PS1=root@server:~# ", "PS2=> ", "PROMPT_COMMAND=", "HISTFILE=/dev/null", "TERM=xterm")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(a.mainCols), Rows: uint16(a.contentRows)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = terminal.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	a.sshPTY = terminal
	chunks := make(chan []byte, 32)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 4096)
		for {
			count, err := terminal.Read(buffer)
			if count > 0 {
				select {
				case chunks <- append([]byte(nil), buffer[:count]...):
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	read := func() {
		t.Helper()
		select {
		case data, ok := <-chunks:
			if !ok {
				t.Fatal("interactive shell exited early")
			}
			a.processSSHOutput(data)
		case <-ctx.Done():
			t.Fatalf("shell timed out: monitor=%v awaitingPrompt=%v", a.inBandMonitor != nil, a.monitorAwaitingPrompt)
		}
	}
	for !a.atPrompt {
		read()
	}
	cursor := a.virtual.Cursor()
	before := monitorScreenText(a)
	history := len(a.terminalHistory)
	for tick := 0; tick < 20; tick++ {
		a.scheduleInBandMonitor(time.Now().Add(time.Duration(tick+1) * time.Minute))
		if a.inBandMonitor == nil {
			t.Fatalf("monitor did not start at tick %d", tick)
		}
		for a.inBandMonitor != nil || a.monitorAwaitingPrompt {
			read()
		}
		after := a.virtual.Cursor()
		if after.X != cursor.X || after.Y != cursor.Y || monitorScreenText(a) != before || len(a.terminalHistory) != history {
			t.Fatalf("tick %d moved the live shell: cursor=(%d,%d) want=(%d,%d), history=%d want=%d", tick, after.X, after.Y, cursor.X, cursor.Y, len(a.terminalHistory), history)
		}
	}
}
