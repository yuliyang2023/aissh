package app

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hinshun/vt10x"
)

var testZmodemHandshake = []byte("**\x18B0100000023be50\r\n\x11")

func TestZmodemHandshakeEntersRawPassthroughAndSuppressesPickerRetry(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 80
	application.contentRows = 22
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	var screen bytes.Buffer
	application.screenOut = &screen

	application.processSSHOutput(append([]byte("rz waiting to receive.\r\n"), testZmodemHandshake...))
	if application.zmodem == nil {
		t.Fatal("ZMODEM passthrough did not start")
	}
	if count := bytes.Count(screen.Bytes(), zmodemStartMarker); count != 1 {
		t.Fatalf("initial handshake count=%d output=%q", count, screen.Bytes())
	}
	if !bytes.Contains(screen.Bytes(), []byte("\x1b[?1049l")) {
		t.Fatalf("alternate screen was not suspended: %q", screen.Bytes())
	}

	application.processSSHOutput(testZmodemHandshake)
	if count := bytes.Count(screen.Bytes(), zmodemStartMarker); count != 1 {
		t.Fatalf("file picker retry leaked to terminal: count=%d output=%q", count, screen.Bytes())
	}

	application.handleInput([]byte("sender-started"))
	application.processSSHOutput(testZmodemHandshake)
	if count := bytes.Count(screen.Bytes(), zmodemStartMarker); count != 2 {
		t.Fatalf("active protocol output was not forwarded: count=%d output=%q", count, screen.Bytes())
	}
}

func TestZmodemMarkerAcrossPTYReadsIsForwardedWhole(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 80
	application.contentRows = 22
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	var screen bytes.Buffer
	application.screenOut = &screen

	application.processSSHOutput([]byte("rz waiting**\x18"))
	application.processSSHOutput([]byte("B0100000023be50\r\n\x11"))

	if application.zmodem == nil {
		t.Fatal("split ZMODEM marker was not detected")
	}
	if count := bytes.Count(screen.Bytes(), zmodemStartMarker); count != 1 {
		t.Fatalf("split handshake was not reconstructed: count=%d output=%q", count, screen.Bytes())
	}
}

func TestZmodemCancelReturnsToTUIAndForwardsCancel(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.mainCols = 80
	application.contentRows = 22
	application.virtual = vt10x.New(vt10x.WithSize(application.mainCols, application.contentRows))
	var screen bytes.Buffer
	application.screenOut = &screen
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	application.sshPTY = writer

	application.processSSHOutput(testZmodemHandshake)
	application.handleInput(zmodemCancelMarker)
	if application.zmodem == nil || application.zmodem.finishSeenAt.IsZero() {
		t.Fatal("cancel did not schedule passthrough exit")
	}
	beforeRetry := bytes.Count(screen.Bytes(), zmodemStartMarker)
	application.processSSHOutput(testZmodemHandshake)
	if afterRetry := bytes.Count(screen.Bytes(), zmodemStartMarker); afterRetry != beforeRetry {
		t.Fatalf("cancelled transfer reopened picker: before=%d after=%d", beforeRetry, afterRetry)
	}
	application.checkZmodemCompletion(time.Now().Add(zmodemFinishDelay + time.Millisecond))
	if application.zmodem != nil {
		t.Fatal("ZMODEM passthrough remained active after cancel")
	}
	if !bytes.Contains(screen.Bytes(), []byte("\x1b[?1049h")) {
		t.Fatalf("TUI was not restored: %q", screen.Bytes())
	}

	forwarded := make([]byte, len(zmodemCancelMarker)+1)
	if _, err := io.ReadFull(reader, forwarded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(forwarded[:len(zmodemCancelMarker)], zmodemCancelMarker) || forwarded[len(forwarded)-1] != '\r' {
		t.Fatalf("cancel/resync bytes=%q", forwarded)
	}
}
