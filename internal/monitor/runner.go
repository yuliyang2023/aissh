package monitor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/sshwrap"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type Runner struct {
	SSHPath     string
	Parsed      sshwrap.Parsed
	ControlPath string
	Timeout     time.Duration
	Language    string
}

// RunError preserves the SSH process exit status while keeping the monitor
// pane's error text concise. OpenSSH uses status 255 for connection and
// authentication failures; remote commands keep their own exit status.
type RunError struct {
	Message           string
	ExitCode          int
	CommandStarted    bool
	ConnectionTimeout bool
}

func (e *RunError) Error() string {
	return e.Message
}

// IsAuthenticationError reports whether an SSH monitor process failed before
// it could run the remote command because authentication was unavailable.
func IsAuthenticationError(err error) bool {
	var runErr *RunError
	if !errors.As(err, &runErr) || runErr.ExitCode != 255 || runErr.CommandStarted {
		return false
	}
	message := strings.ToLower(runErr.Message)
	for _, marker := range []string{
		"permission denied",
		"authentication failed",
		"unable to authenticate",
		"no supported authentication methods available",
		"too many authentication failures",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func CanRetryViaPrimary(err error) bool {
	var runErr *RunError
	return IsAuthenticationError(err) || (errors.As(err, &runErr) && runErr.ConnectionTimeout && !runErr.CommandStarted)
}

func (r Runner) Run(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("monitor command is empty")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	queryContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}
	if r.ControlPath != "" {
		args = append(args, "-o", "ControlPath="+r.ControlPath)
	}
	args = append(args, r.Parsed.MonitorArgs()...)
	// A private startup marker distinguishes SSH setup timeouts from slow
	// commands. Only setup failures may retry through the authenticated shell.
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	marker := "AISSH_MONITOR_START_" + hex.EncodeToString(token[:])
	wrapped := "printf '\\036" + marker + "\\037'; exec sh -c '" + strings.ReplaceAll(command, "'", "'\"'\"'") + "'"
	args = append(args, wrapped)
	cmd := exec.CommandContext(queryContext, r.SSHPath, args...)
	configureMonitorProcess(cmd)
	cmd.WaitDelay = time.Second
	data, err := cmd.CombinedOutput()
	startMarker := []byte("\x1e" + marker + "\x1f")
	started := bytes.Contains(data, startMarker)
	data = bytes.Replace(data, startMarker, nil, 1)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if queryContext.Err() != nil {
		return "", &RunError{Message: "monitor query timed out", ExitCode: -1, CommandStarted: started, ConnectionTimeout: !started}
	}
	if err != nil {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = err.Error()
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return "", &RunError{
			Message: message, ExitCode: exitCode, CommandStarted: started,
			ConnectionTimeout: !started && exitCode == 255 && strings.Contains(strings.ToLower(message), "timed out"),
		}
	}
	return r.Format(command, strings.TrimSpace(string(data))), nil
}

// Format applies the same presentation rules to output collected through an
// already authenticated interactive SSH session.
func (r Runner) Format(command, output string) string {
	return formatBuiltin(command, strings.TrimSpace(output), r.Language)
}

func formatBuiltin(command, output, language string) string {
	if command != panel.BuiltinOverviewCommand {
		return output
	}
	labels := map[string]string{
		"HOST":   "主机",
		"UPTIME": "运行",
		"LOAD":   "负载",
		"MEM":    "内存",
		"DISK":   "根分区",
		"PROCS":  "进程数",
	}
	if strings.HasPrefix(strings.ToLower(language), "en") {
		labels = map[string]string{
			"HOST":   "Host",
			"UPTIME": "Uptime",
			"LOAD":   "Load",
			"MEM":    "Memory",
			"DISK":   "Root disk",
			"PROCS":  "Processes",
		}
	}
	var rows [][2]string
	labelWidth := 0
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		label := labels[parts[0]]
		if label == "" {
			label = parts[0]
		}
		if width := cellWidth(label); width > labelWidth {
			labelWidth = width
		}
		rows = append(rows, [2]string{label, strings.TrimSpace(parts[1])})
	}
	var lines []string
	for _, row := range rows {
		padding := strings.Repeat(" ", labelWidth-cellWidth(row[0])+1)
		lines = append(lines, row[0]+padding+row[1])
	}
	return strings.Join(lines, "\n")
}

// cellWidth measures display width in terminal cells; CJK labels occupy
// two cells per rune, which fmt's %-8s would miscount.
func cellWidth(s string) int {
	cells := 0
	for _, r := range s {
		cells += terminalview.CellWidth(r)
	}
	return cells
}
