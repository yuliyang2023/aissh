//go:build !windows

package desktop

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
)

func TestTerminalBridgeInputResizeAndExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'bridge-ready\\n'\nread value\nprintf 'received:%s\\n' \"$value\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AISSH_CLI_PATH", path)
	store := catalog.New(t.TempDir(), memorySecrets{})
	host, err := store.SaveHost(catalog.Host{Name: "test", Address: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	s := New(store)
	t.Cleanup(func() { Shutdown(s) })
	events := make(chan string, 32)
	exited := make(chan struct{}, 1)
	s.emit = func(name string, args ...any) {
		if strings.HasPrefix(name, "terminal:") {
			data, _ := base64.StdEncoding.DecodeString(args[0].(string))
			events <- string(data)
		} else if strings.HasPrefix(name, "terminal-exit:") {
			exited <- struct{}{}
		}
	}
	session, err := s.CreateSession(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartSession(session.ID, 100, 30); err != nil {
		t.Fatal(err)
	}
	if err := s.ResizeTerminal(session.ID, 120, 40); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteTerminal(session.ID, base64.StdEncoding.EncodeToString([]byte("hello-桥接\n"))); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal exit was not delivered")
	}
	var output strings.Builder
	for len(events) > 0 {
		output.WriteString(<-events)
	}
	if !strings.Contains(output.String(), "bridge-ready") || !strings.Contains(output.String(), "received:hello-桥接") {
		t.Fatalf("terminal output: %q", output.String())
	}
	if err := s.WriteTerminal(session.ID, "aA=="); err == nil {
		t.Fatal("closed terminal accepted input")
	}
	if err := s.CloseSession(session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.session(session.ID); err == nil {
		t.Fatal("closed session retained")
	}
}

func TestSessionStartupFailureReleasesCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AISSH_CLI_PATH", path)
	store := catalog.New(t.TempDir(), memorySecrets{})
	host, err := store.SaveHost(catalog.Host{Name: "test", Address: "localhost", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	s := New(store)
	t.Cleanup(func() { Shutdown(s) })
	for i := 0; i < 13; i++ {
		session, err := s.CreateSession(host.ID)
		if err != nil {
			t.Fatalf("failed sessions consumed capacity: %v", err)
		}
		if err := s.StartSession(session.ID, 80, 24); err == nil {
			t.Fatal("non-executable CLI started")
		}
	}
	for _, session := range s.ListSessions() {
		if session.Status != "failed" || session.EndedAt == "" {
			t.Fatalf("incorrect failure state: %+v", session)
		}
	}
}

func TestSessionLimitAndEndedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test-cli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AISSH_CLI_PATH", path)
	store := catalog.New(t.TempDir(), memorySecrets{})
	host, err := store.SaveHost(catalog.Host{Name: "test", Address: "localhost", Port: 2222, User: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	s := New(store)
	t.Cleanup(func() { Shutdown(s) })
	for i := 0; i < 12; i++ {
		if _, err := s.CreateSession(host.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateSession(host.ID); err == nil {
		t.Fatal("allowed more than 12 live sessions")
	}
	first := s.ListSessions()[0]
	if first.Status != "starting" || first.Address != host.Address || first.Port != 2222 || first.User != "ops" || first.CreatedAt == "" {
		t.Fatalf("missing session metadata: %+v", first)
	}
	done := make(chan struct{}, 1)
	s.emit = func(name string, args ...any) {
		if name == "terminal-exit:"+first.ID {
			done <- struct{}{}
		}
	}
	if err := s.StartSession(first.ID, 80, 24); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit")
	}
	ended := s.ListSessions()[0]
	if ended.Status != "ended" || ended.EndedAt == "" {
		t.Fatalf("missing ended state: %+v", ended)
	}
	if _, err := s.CreateSession(host.ID); err != nil {
		t.Fatalf("ended tab consumed active slot: %v", err)
	}
	if len(s.ListSessions()) != 13 {
		t.Fatal("ended terminal metadata was not retained")
	}
}
