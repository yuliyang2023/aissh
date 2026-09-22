package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type shortSSHWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *shortSSHWriter) Write(data []byte) (int, error) {
	n, _ := w.Buffer.Write(data[:min(len(data), w.limit)])
	return n, w.err
}

func TestSSHWriterHandlesShortWritesAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		err   error
		want  error
	}{
		{"short writes", 7, nil, nil},
		{"zero progress", 0, nil, io.ErrShortWrite},
		{"partial failure", 7, io.ErrClosedPipe, io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			target := &shortSSHWriter{limit: tc.limit, err: tc.err}
			events := make(chan interface{}, 4)
			w := newSSHInputWriter(ctx, target, events)
			data := strings.Repeat("payload", 1000)
			job := w.enqueue([]byte(data), "test", false)
			select {
			case value := <-events:
				event := value.(sshWriteEvent)
				if !errors.Is(event.err, tc.want) || event.written != target.Len() || !job.done.Load() {
					t.Fatalf("write event=%+v actual=%d", event, target.Len())
				}
				if tc.want == nil && target.String() != data {
					t.Fatal("short writes lost or duplicated bytes")
				}
			case <-time.After(time.Second):
				t.Fatal("write stalled")
			}
		})
	}
}

type gatedSSHWriter struct {
	target  io.Writer
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (w *gatedSSHWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.calls == 2 {
		close(w.entered)
		<-w.release
	}
	return w.target.Write(data)
}

func awaitExecutionWrite(t *testing.T, a *App) {
	t.Helper()
	deadline := time.After(time.Second)
	for a.execution != nil && a.execution.WriteJob != nil {
		select {
		case value := <-a.events:
			a.handleEvent(value)
			if event, ok := value.(sshWriteEvent); ok && !event.job.interrupt {
				return
			}
		case <-deadline:
			t.Fatal("execution write timed out")
		}
	}
}

func TestCancelBlockedCommandStopsSuffixAndKeepsUIResponsive(t *testing.T) {
	a, _ := executionTestApp(t)
	var terminal bytes.Buffer
	gate := &gatedSSHWriter{target: &terminal, entered: make(chan struct{}), release: make(chan struct{})}
	a.sshInput = newSSHInputWriter(a.ctx, gate, a.events)
	a.execution.Command = strings.Repeat("x", 100000) + "\nprintf MUST_NOT_RUN"
	a.sendApprovedCommand(a.execution)
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("write did not reach gate")
	}
	// These calls run on the UI loop even while a transport write is blocked.
	a.handleInput([]byte{3, 3})
	a.handleInput([]byte("printf NEXT_COMMAND\r"))
	if !a.executionRecovery || a.atPrompt || a.execution.Completion != "cancelled" {
		t.Fatalf("cancellation did not release local waiting: %+v", a.execution)
	}
	a.processSSHOutput([]byte("root@server:~# "))
	if a.atPrompt {
		t.Fatal("enabled input while cancelled bytes were still in flight")
	}
	close(gate.release)
	deadline := time.After(time.Second)
	for a.executionInterrupt != nil {
		select {
		case event := <-a.events:
			a.handleEvent(event)
			if event, ok := event.(sshWriteEvent); ok && event.job.interrupt {
				if event.err != nil {
					t.Fatal(event.err)
				}
				goto drained
			}
		case <-deadline:
			t.Fatal("interrupt did not drain")
		}
	}
drained:
	if a.atPrompt || !a.executionRecovery {
		t.Fatal("output before the interrupt was accepted as a fresh prompt")
	}
	a.processSSHOutput([]byte("\r\nroot@server:~# "))
	if !a.atPrompt || a.executionRecovery {
		t.Fatal("fresh prompt after the interrupt did not recover")
	}
	got := terminal.String()
	if len(got) > 2*sshWriteChunkSize+2 || !strings.HasSuffix(got, "\x03\x03") || strings.Contains(got, "NEXT_COMMAND") {
		t.Fatalf("cancelled command or queued typing leaked: %q", got)
	}
}
