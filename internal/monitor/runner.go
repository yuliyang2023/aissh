package monitor

import (
	"context"
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
	args := []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}
	if r.ControlPath != "" {
		args = append(args, "-o", "ControlPath="+r.ControlPath)
	}
	args = append(args, r.Parsed.MonitorArgs()...)
	args = append(args, command)
	cmd := exec.CommandContext(queryContext, r.SSHPath, args...)
	data, err := cmd.CombinedOutput()
	if queryContext.Err() != nil {
		return "", fmt.Errorf("monitor query timed out")
	}
	if err != nil {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	return formatBuiltin(command, strings.TrimSpace(string(data)), r.Language), nil
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
