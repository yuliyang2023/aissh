//go:build !windows

package monitor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/sshwrap"
)

func TestRunnerDistinguishesConnectionAndCommandTimeout(t *testing.T) {
	for _, connected := range []bool{false, true} {
		name := "connection"
		if connected {
			name = "command"
		}
		t.Run(name, func(t *testing.T) {
			script := "#!/bin/sh\nexec sleep 30\n"
			if connected {
				script = "#!/bin/sh\nfor arg do command=$arg; done\nexec sh -c \"$command\"\n"
			}
			ssh := filepath.Join(t.TempDir(), "ssh")
			if err := os.WriteFile(ssh, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			// Leave enough time for the temporary executable to start under a
			// loaded test runner, so the command case actually reaches its marker.
			runner := Runner{SSHPath: ssh, Parsed: sshwrap.Parsed{Target: "server"}, Timeout: 5 * time.Second}
			_, err := runner.Run(context.Background(), "exec sleep 30")
			var runErr *RunError
			if !errors.As(err, &runErr) || runErr.CommandStarted != connected || CanRetryViaPrimary(err) == connected {
				t.Fatalf("connected=%v error=%#v fallback=%v", connected, err, CanRetryViaPrimary(err))
			}
		})
	}
}

func TestRunnerRemovesStartupMarkerAndPreservesShellQuoting(t *testing.T) {
	ssh := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(ssh, []byte("#!/bin/sh\nfor arg do command=$arg; done\nexec sh -c \"$command\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	runner := Runner{SSHPath: ssh, Parsed: sshwrap.Parsed{Target: "server"}}
	output, err := runner.Run(context.Background(), "printf '%s\\n' \"it's ready\"")
	if err != nil || output != "it's ready" || strings.Contains(output, "AISSH_MONITOR") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}
