package history

import (
	"os"
	"strings"
	"testing"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestStoreSeparatesTargetsAndRoundTripsProposal(t *testing.T) {
	store := New(t.TempDir())
	first := Message{
		Role:    "user",
		Content: "检查 nginx",
		Time:    "2026-08-25T10:00:00+08:00",
	}
	second := Message{
		Role:    "proposal",
		Content: "检查服务状态",
		Time:    "2026-08-25T10:01:00+08:00",
		Proposal: &agent.Proposal{
			ID: "p1", Command: "systemctl status nginx", Risk: "read_only",
		},
	}
	if err := store.Append("root@server-a", first); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("root@server-a", second); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("root@server-b", Message{Role: "assistant", Content: "另一台服务器"}); err != nil {
		t.Fatal(err)
	}

	messages, err := store.Load("root@server-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != first.Content || messages[1].Proposal == nil || messages[1].Proposal.Command != second.Proposal.Command {
		t.Fatalf("messages=%#v", messages)
	}
	info, err := os.Stat(store.Path("root@server-a"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("history permissions=%o", info.Mode().Perm())
	}
}

func TestStoreGroupsConversationsAndKeepsLegacyHistory(t *testing.T) {
	store := New(t.TempDir())
	for _, message := range []Message{
		{ConversationID: "older", Role: "user", Content: "检查 nginx", Time: "2026-08-24T10:00:00+08:00"},
		{ConversationID: "newer", Role: "user", Content: "查看磁盘空间", Time: "2026-08-25T10:00:00+08:00"},
		{ConversationID: "newer", Role: "assistant", Content: "磁盘正常", Time: "2026-08-25T10:01:00+08:00"},
		{Role: "user", Content: "旧版历史问题", Time: "2026-08-23T10:00:00+08:00"},
	} {
		if err := store.Append("server", message); err != nil {
			t.Fatal(err)
		}
	}

	conversations, err := store.LoadConversations("server")
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 3 || conversations[0].ID != "newer" || conversations[0].Title != "查看磁盘空间" || len(conversations[0].Messages) != 2 {
		t.Fatalf("conversations=%#v", conversations)
	}
	if conversations[2].ID != LegacyConversationID || conversations[2].Title != "旧版历史问题" {
		t.Fatalf("legacy=%#v", conversations[2])
	}
	if conversations[0].UpdatedAt.Sub(conversations[0].StartedAt) != time.Minute {
		t.Fatalf("conversation times=%s..%s", conversations[0].StartedAt, conversations[0].UpdatedAt)
	}
}

func TestStoreLoadsLatestEntriesAndCompacts(t *testing.T) {
	store := New(t.TempDir())
	store.MaxEntries = 3
	store.MaxFileSize = 1
	for _, content := range []string{"one", "two", "three", "four"} {
		if err := store.Append("server", Message{Role: "assistant", Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := store.Load("server")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || strings.Join([]string{messages[0].Content, messages[1].Content, messages[2].Content}, ",") != "two,three,four" {
		t.Fatalf("messages=%#v", messages)
	}
}
