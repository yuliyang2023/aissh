package desktop

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
)

func TestWorkbenchChannelAuthenticationSessionIsolationAndAcknowledgement(t *testing.T) {
	s, host, _ := conversationService(t)
	info, err := s.CreateSession(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	session, _ := s.session(info.ID)
	env, closeChannel, err := s.credentialChannel(session)
	if err != nil {
		t.Fatal(err)
	}
	defer closeChannel()
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		t.Setenv(key, value)
	}
	req, _ := http.NewRequest(http.MethodPost, "http://"+os.Getenv(credentialui.AddressEnv)+"/workbench", strings.NewReader("{}"))
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal("unauthenticated snapshot accepted")
	}
	state := credentialui.WorkbenchState{ConversationID: "conversation-a", Status: "ready"}
	if _, err := credentialui.ExchangeWorkbench(context.Background(), state, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.GetWorkbench(info.ID)
	if err != nil || snapshot.Status != "ready" || snapshot.UpdatedAt == "" {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	second, _ := s.CreateSession(host.ID)
	if snapshot, _ := s.GetWorkbench(second.ID); snapshot != nil {
		t.Fatal("snapshot leaked across sessions")
	}
	result := make(chan error, 1)
	go func() {
		result <- s.WorkbenchAction(info.ID, credentialui.WorkbenchCommand{Kind: "prompt", Text: "检查系统"})
	}()
	var commands []credentialui.WorkbenchCommand
	deadline := time.Now().Add(time.Second)
	for len(commands) == 0 && time.Now().Before(deadline) {
		commands, err = credentialui.ExchangeWorkbench(context.Background(), state, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(commands) == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(commands) != 1 || commands[0].Text != "检查系统" || commands[0].Deadline <= time.Now().UnixMilli() {
		t.Fatalf("commands: %+v", commands)
	}
	if _, err := credentialui.ExchangeWorkbench(context.Background(), state, []credentialui.CommandResult{{ID: commands[0].ID, Error: "AI is busy"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil || err.Error() != "AI is busy" {
		t.Fatalf("lost command acknowledgement: %v", err)
	}
	commands, err = credentialui.ExchangeWorkbench(context.Background(), state, nil)
	if err != nil || len(commands) != 0 {
		t.Fatal("command delivered twice")
	}
	session.finish("ended")
	if err := s.WorkbenchAction(info.ID, credentialui.WorkbenchCommand{Kind: "prompt"}); err == nil {
		t.Fatal("closed session accepted action")
	}
}
