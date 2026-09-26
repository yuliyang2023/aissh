package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/history"
	"git.example.com/ai/wrd-ai-cli/internal/redact"
)

func TestImportAndSearchGenericKnowledgeTables(t *testing.T) {
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	for _, text := range []string{
		"\ufeff资产,部门,备注\r\n服务器,研发,\"包含逗号,和\n换行\"\r\n",
		"\r\n资产\t部门\t备注\n服务器\t研发\t\"包含逗号,和\n换行\"\n",
	} {
		k, err := s.ParseKnowledgeTable(text)
		if err != nil || len(k.Columns) != 3 || len(k.Rows) != 1 || k.Rows[0][2] != "包含逗号,和\n换行" {
			t.Fatalf("import: %+v %v", k, err)
		}
		if _, err := s.SaveKnowledge(k); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.ListKnowledge("研发")
	if err != nil || len(items) != 2 {
		t.Fatalf("cell search: %+v %v", items, err)
	}
	for _, text := range []string{"", "名称,名称\na,b", "名称,部门\na,b,c", "名称,\na,b"} {
		if _, err := s.ParseKnowledgeTable(text); err == nil {
			t.Errorf("accepted invalid input %q", text)
		}
	}
	quotedHeader, err := s.ParseKnowledgeTable("\"资产\t名称\",部门\n服务器,研发")
	if err != nil || len(quotedHeader.Columns) != 2 || quotedHeader.Columns[0] != "资产\t名称" {
		t.Fatalf("quoted tab was mistaken for a delimiter: %+v %v", quotedHeader, err)
	}
}

func TestDocumentReferencesDoNotModifyOriginals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "说明.md")
	content := []byte("# 文档\n本地资料")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	k, err := knowledgeDocument(path)
	if err != nil || k.Content != string(content) || k.FilePath != path {
		t.Fatalf("document: %+v %v", k, err)
	}
	s := New(catalog.New(t.TempDir(), memorySecrets{}))
	k, err = s.SaveKnowledge(k)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteKnowledge(k.ID, k.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(content) {
		t.Fatal("deleting reference altered original document")
	}
	if _, err := knowledgeDocument(filepath.Join(dir, "missing.pdf")); err == nil {
		t.Fatal("accepted missing document")
	}
	if _, err := knowledgeDocument(filepath.Join(dir, "script.sh")); err == nil {
		t.Fatal("accepted executable document")
	}
}

type knowledgeProvider struct {
	turn    agent.Turn
	request agent.Request
}

func (p *knowledgeProvider) Name() string     { return "test" }
func (p *knowledgeProvider) Configured() bool { return true }
func (p *knowledgeProvider) Stream(_ context.Context, r agent.Request, _ agent.Callbacks) (agent.Turn, error) {
	p.request = r
	return p.turn, nil
}
func (p *knowledgeProvider) Action(context.Context, agent.Request) (agent.Turn, error) {
	panic("summary must never execute actions")
}

func TestSummaryDraftPreservesSourceAndRejectsProposals(t *testing.T) {
	c := history.Conversation{ID: "2026.09-test", Title: "服务排障", Messages: []history.Message{{Role: "system", Content: "hidden"}, {Role: "user", Content: "检查 secret-test-value"}, {Role: "assistant", Content: "建议检查日志；结果未确认"}}}
	p := &knowledgeProvider{turn: agent.Turn{Type: "finished", Message: "经验 secret-test-value"}}
	k, err := summaryDraft(context.Background(), p, c, "host", redact.New("secret-test-value"))
	if err != nil || k.ConversationID != c.ID || k.HostID != "host" || k.ID != "" || !strings.Contains(k.Content, "[redacted]") {
		t.Fatalf("draft: %+v %v", k, err)
	}
	if strings.Contains(p.request.Prompt, "secret-test-value") || strings.Contains(p.request.Prompt, "hidden") || p.request.SessionID != "" {
		t.Fatal("summary leaked secret or reused live context")
	}
	p.turn = agent.Turn{Type: "command_proposal", Proposal: &agent.Proposal{Command: "rm -rf /"}, Message: "执行这个"}
	if _, err := summaryDraft(context.Background(), p, c, "host", redact.New()); err == nil {
		t.Fatal("accepted command proposal as summary")
	}
	p.turn = agent.Turn{Type: "finished", Message: ""}
	if _, err := summaryDraft(context.Background(), p, c, "host", redact.New()); err == nil {
		t.Fatal("accepted empty summary")
	}
}

func TestSummarizeConversationUsesConfiguredAIWithoutOpeningSSH(t *testing.T) {
	s, host, other := conversationService(t)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "summary-test" {
			t.Errorf("wrong model: %v", body["model"])
		}
		response, _ := json.Marshal(map[string]any{"type": "finished", "message": "问题已确认，建议仍待验证", "proposal": nil, "panelActions": []any{}})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(response)}}}})
	}))
	defer server.Close()
	if _, err := s.SaveAIProfile(catalog.AIProfile{Name: "总结模型", Provider: "model", URL: server.URL, Model: "summary-test"}, true); err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateConversation(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.history.Append("profile:"+host.ID, history.Message{ConversationID: c.ID, Role: "user", Content: "请检查服务"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SummarizeConversation(other.ID, c.ID); err == nil || called {
		t.Fatal("cross-host history sent to AI")
	}
	k, err := s.SummarizeConversation(host.ID, c.ID)
	if err != nil || !called || k.Content != "问题已确认，建议仍待验证" {
		t.Fatalf("summary: %+v %v", k, err)
	}
	items, _ := s.ListKnowledge("")
	if len(items) != 0 || len(s.ListSessions()) != 0 {
		t.Fatal("generating draft persisted knowledge or opened SSH")
	}
}
