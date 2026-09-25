//go:build !windows

package app

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSSHExitWaitsForFinalOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		keepOpen bool
	}{
		{"delayed diagnostic", false},
		{"pipe remains open after process exit", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := New(Config{SSHArgs: []string{"server"}})
			if err != nil {
				t.Fatal(err)
			}
			defer a.cancel()
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			defer writer.Close()
			a.sshPTY = reader
			a.sshCommand = exec.CommandContext(a.ctx, "sh", "-c", "exit 255")
			if err := a.sshCommand.Start(); err != nil {
				t.Fatal(err)
			}
			a.startOutputReader()
			// Model final output reaching the pipe shortly after process exit.
			select {
			case event := <-a.events:
				t.Fatalf("exit overtook unread output: %#v", event)
			case <-time.After(50 * time.Millisecond):
			}
			const diagnostic = "ssh: connect to host server port 2222: Connection refused\r\n"
			if _, err := io.WriteString(writer, diagnostic); err != nil {
				t.Fatal(err)
			}
			if !tc.keepOpen {
				_ = writer.Close()
			}
			var output strings.Builder
			timeout := time.NewTimer(3 * time.Second)
			defer timeout.Stop()
			for {
				select {
				case event := <-a.events:
					switch value := event.(type) {
					case outputEvent:
						output.Write(value.data)
					case sshExitEvent:
						var exitErr *exec.ExitError
						if !errors.As(value.err, &exitErr) || exitErr.ExitCode() != 255 {
							t.Fatalf("lost SSH process exit status: %v", value.err)
						}
						if output.String() != diagnostic {
							t.Fatalf("exit discarded final output: %q", output.String())
						}
						return
					}
				case <-timeout.C:
					t.Fatal("open output pipe prevented SSH exit")
				}
			}
		})
	}
}
