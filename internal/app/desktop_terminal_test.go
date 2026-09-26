package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDesktopTerminalStreamsOriginalSSHBytes(t *testing.T) {
	a := desktopTestApp(t)
	var output bytes.Buffer
	a.screenOut = &output
	data := []byte(strings.Repeat("\x1b[32m终端输出\x1b[0m\r\n", 120) +
		"\x1b[?1049h\x1b[?1006h\x1b[?1000hremote editor" +
		"\x1b[?1000l\x1b[?1006l\x1b[?1049lroot@server:~# ")
	// Split UTF-8 and escape sequences across reads, as a real PTY does.
	for start := 0; start < len(data); start += 7 {
		a.processSSHOutput(data[start:min(start+7, len(data))])
		a.render()
	}
	if !bytes.Equal(output.Bytes(), data) {
		t.Fatal("desktop output lost bytes or added a CLI repaint")
	}
	if !a.atPrompt || !a.safePromptScreen() {
		t.Fatal("the observer lost the shell prompt after the remote editor exited")
	}
}

func TestDesktopTerminalInputBelongsToRemoteShell(t *testing.T) {
	for _, data := range []string{
		"中文命令\r", "/ai literal-shell-input\r", "/sidebar show\r",
		"\x1b[<0;12;5M\x1b[<32;12;6M\x1b[<0;12;6m", // remote mouse press/drag/release
		"\x1d\x0f\x1f", // CLI-local shortcuts must reach the remote terminal
		"\x1b[200~中文粘贴\x1b[201~",
	} {
		t.Run(data, func(t *testing.T) {
			a := desktopTestApp(t)
			a.atPrompt = true
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			a.sshPTY = writer
			a.handleInput([]byte(data))
			writer.Close()
			written, err := io.ReadAll(reader)
			if err != nil || string(written) != data {
				t.Fatalf("SSH input=%q, want %q, error=%v", written, data, err)
			}
			if a.aiLineCapture || a.agentBusy || a.mouseMode || a.inlineAI != nil {
				t.Fatal("desktop terminal input activated a CLI interaction")
			}
		})
	}
}

func TestDesktopTerminalHidesExecutionAndMonitorProtocol(t *testing.T) {
	t.Run("execution", func(t *testing.T) {
		a, _ := executionTestApp(t)
		a.config.DesktopUI = true
		var output bytes.Buffer
		a.screenOut = &output
		data := "RESULT\r\n" + executionMarkerPrefix("test") + "0\a" + "root@server:~# "
		for _, b := range []byte(data) {
			a.processSSHOutput([]byte{b})
		}
		if got := output.String(); got != "RESULT\r\nroot@server:~# " {
			t.Fatalf("execution protocol appeared in desktop output: %q", got)
		}
	})
	t.Run("monitor", func(t *testing.T) {
		a := desktopTestApp(t)
		var output bytes.Buffer
		a.screenOut = &output
		const marker = "__AISSH_MONITOR_TEST_"
		a.inBandMonitor = &inBandMonitorState{panelID: "system", marker: marker}
		a.processSSHOutput([]byte(marker + "BEGIN__\r\nHOST=hidden-monitor\r\n" + marker + "END__:0__\r\nroot@server:~# "))
		if strings.Contains(output.String(), marker) || strings.Contains(output.String(), "hidden-monitor") {
			t.Fatalf("monitor protocol appeared in desktop output: %q", output.String())
		}
		if !strings.Contains(output.String(), "root@server:~# ") {
			t.Fatalf("shell prompt missing after monitor: %q", output.String())
		}
	})
}

func TestDesktopTerminalTransferDoesNotResetScrollback(t *testing.T) {
	a := desktopTestApp(t)
	var output bytes.Buffer
	a.screenOut = &output
	// A handshake split over two reads must reach the client exactly once.
	a.processSSHOutput(testZmodemHandshake[:4])
	a.processSSHOutput(testZmodemHandshake[4:])
	if !bytes.Equal(output.Bytes(), testZmodemHandshake) {
		t.Fatalf("transfer output=%q", output.Bytes())
	}
	a.leaveZmodemPassthrough()
	a.render()
	if !bytes.Equal(output.Bytes(), testZmodemHandshake) {
		t.Fatal("transfer exit reset the desktop terminal or its scrollback")
	}
}
