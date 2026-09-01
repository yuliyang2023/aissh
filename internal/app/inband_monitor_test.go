package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInBandMonitorCommandKeepsMarkersOutOfShellEcho(t *testing.T) {
	marker := "__AISSH_MONITOR_7_"
	command := inBandMonitorCommand(marker, "printf '%s' \"it's safe\"")
	if strings.Contains(command, marker+"BEGIN__") || strings.Contains(command, marker+"END__:") {
		t.Fatalf("complete monitor marker leaked into echoed command: %q", command)
	}
	if !strings.HasSuffix(command, "\r") {
		t.Fatalf("monitor command does not submit the shell line: %q", command)
	}
}

func TestInBandMonitorStateCollectsChunkedOutputAndRestoresPrompt(t *testing.T) {
	state := &inBandMonitorState{marker: "__AISSH_MONITOR_9_"}
	chunks := [][]byte{
		[]byte("echoed command containing __AISSH_MONITOR_9_ but no complete marker\r\n__AISSH_MON"),
		[]byte("ITOR_9_BEGIN__\r\nHOST=server\r\nUPTIME=1 day\r\n"),
		[]byte("\r\n__AISSH_MONITOR_9_END__:0\x1b[2"),
		[]byte("K\ruser@server$ "),
	}
	for index, chunk := range chunks {
		visible, complete, content, err := state.consume(chunk)
		if index < len(chunks)-1 {
			if complete || len(visible) != 0 || content != "" || err != nil {
				t.Fatalf("chunk %d completed early: visible=%q complete=%v content=%q err=%v", index, visible, complete, content, err)
			}
			continue
		}
		if err != nil || !complete {
			t.Fatalf("final chunk complete=%v err=%v", complete, err)
		}
		if got, want := content, "HOST=server\r\nUPTIME=1 day\r\n"; got != want {
			t.Fatalf("content=%q want=%q", got, want)
		}
		if got, want := string(visible), "\x1b[2K\ruser@server$ "; got != want {
			t.Fatalf("visible=%q want=%q", got, want)
		}
	}
}

func TestInBandMonitorStateReportsRemoteFailure(t *testing.T) {
	state := &inBandMonitorState{marker: "__AISSH_MONITOR_11_"}
	data := []byte("__AISSH_MONITOR_11_BEGIN__\r\npermission denied\r\n\r\n__AISSH_MONITOR_11_END__:1\x1b[2K\rhost$ ")
	visible, complete, content, err := state.consume(data)
	if !complete || err == nil || content != "" {
		t.Fatalf("complete=%v content=%q err=%v", complete, content, err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("error=%q", err)
	}
	if !bytes.HasPrefix(visible, inBandMonitorScreenRestore) {
		t.Fatalf("prompt restore missing: %q", visible)
	}
}

func TestScheduleInBandMonitorUsesPrimaryPTY(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	application.sshPTY = writer
	application.monitorViaPrimary = true
	application.atPrompt = true
	application.lineReliable = true

	application.scheduleMonitors()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if application.inBandMonitor == nil || !application.monitorActive["system"] || application.atPrompt {
		t.Fatalf("state=%#v active=%v atPrompt=%v", application.inBandMonitor, application.monitorActive["system"], application.atPrompt)
	}
	if !bytes.Contains(written, []byte("sh -c")) || !bytes.HasSuffix(written, []byte{'\r'}) {
		t.Fatalf("primary PTY command=%q", written)
	}
}

func TestInputWaitsForActiveInBandMonitor(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.inBandMonitor = &inBandMonitorState{startedAt: time.Now()}
	application.handleInput([]byte("whoami\r"))
	if got, want := string(application.inBandMonitor.pendingInput), "whoami\r"; got != want {
		t.Fatalf("pending input=%q want=%q", got, want)
	}
}

func TestInBandMonitorTimeoutInterruptsQueryAndPreservesInput(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	application.sshPTY = writer
	application.monitor.Timeout = time.Second
	application.monitorActive["system"] = true
	application.inBandMonitor = &inBandMonitorState{
		panelID:      "system",
		startedAt:    time.Now().Add(-2 * time.Second),
		pendingInput: []byte("date\r"),
	}

	application.checkInBandMonitorTimeout(time.Now())
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := written, []byte{0x03}; !bytes.Equal(got, want) {
		t.Fatalf("interrupt=%v want=%v", got, want)
	}
	if application.inBandMonitor != nil || application.monitorActive["system"] {
		t.Fatalf("monitor was not released: state=%#v active=%v", application.inBandMonitor, application.monitorActive["system"])
	}
	if got, want := string(application.monitorDeferredInput), "date\r"; got != want {
		t.Fatalf("deferred input=%q want=%q", got, want)
	}
}
