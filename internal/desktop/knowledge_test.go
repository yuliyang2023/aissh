package desktop

import (
	"context"
	"encoding/json"
	"errors"
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
	turn     agent.Turn
	request  agent.Request
	requests []agent.Request
	reply    func(agent.Callbacks) (agent.Turn, error)
}

func (p *knowledgeProvider) Name() string     { return "test" }
func (p *knowledgeProvider) Configured() bool { return true }
func (p *knowledgeProvider) Stream(_ context.Context, r agent.Request, cb agent.Callbacks) (agent.Turn, error) {
	p.request = r
	p.requests = append(p.requests, r)
	if p.reply != nil {
		return p.reply(cb)
	}
	return p.turn, nil
}
func (p *knowledgeProvider) Action(context.Context, agent.Request) (agent.Turn, error) {
	panic("summary must never execute actions")
}

func TestSummaryDraftRetriesInvalidResponsesWithoutExecutingActions(t *testing.T) {
	conversation := history.Conversation{ID: "source", Title: "排障", Messages: []history.Message{{Role: "user", Content: "历史资料 secret-test-value"}}}
	for _, tc := range []struct {
		name      string
		turn      agent.Turn
		panel     bool
		wantError string
	}{
		{"question", agent.Turn{Type: "ask_user", Message: "请先执行命令"}, false, "追问"},
		{"proposal", agent.Turn{Type: "command_proposal", Proposal: &agent.Proposal{Command: "uptime"}}, false, "操作请求"},
		{"finished_with_proposal", agent.Turn{Type: "finished", Message: "总结", Proposal: &agent.Proposal{Command: "uptime"}}, false, "操作请求"},
		{"panel", agent.Turn{Type: "finished", Message: "已打开面板"}, true, "操作请求"},
		{"empty", agent.Turn{Type: "finished", Message: " \n "}, false, "内容为空"},
		{"unknown", agent.Turn{Type: "unknown", Message: "总结"}, false, "未按要求"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, succeedsOnRetry := range []bool{true, false} {
				p := &knowledgeProvider{}
				p.reply = func(cb agent.Callbacks) (agent.Turn, error) {
					if succeedsOnRetry && len(p.requests) == 2 {
						return agent.Turn{Type: "finished", Message: "经验 secret-test-value"}, nil
					}
					if tc.panel && cb.OnPanelAction != nil {
						cb.OnPanelAction(agent.PanelAction{Action: "create"})
					}
					return tc.turn, nil
				}
				k, err := summaryDraft(context.Background(), p, conversation, "host", redact.New("secret-test-value"))
				if len(p.requests) != 2 {
					t.Fatalf("expected one retry, got %d requests", len(p.requests))
				}
				for _, r := range p.requests {
					if r.Event != "start" || r.SessionID != "" || r.SystemContext["conversation_summary"] != "true" || !strings.Contains(r.Prompt, "历史资料 [redacted]") || strings.Contains(r.Prompt, "secret-test-value") {
						t.Fatal("retry lost source, leaked secret or reused a live session")
					}
				}
				if succeedsOnRetry {
					if err != nil || k.Content != "经验 [redacted]" || k.ConversationID != conversation.ID || k.HostID != "host" || k.ID != "" {
						t.Fatalf("recovered draft: %+v %v", k, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.wantError) || k.Content != "" {
					t.Fatalf("invalid response was accepted or misreported: %+v %v", k, err)
				}
			}
		})
	}
}

func TestSummaryDraftDoesNotRetryProviderErrorsOrCancellation(t *testing.T) {
	conversation := history.Conversation{Messages: []history.Message{{Role: "user", Content: "资料"}}}
	for _, errorTurn := range []bool{false, true} {
		p := &knowledgeProvider{reply: func(agent.Callbacks) (agent.Turn, error) {
			if errorTurn {
				return agent.Turn{Type: "error", Error: "API secret-test-value"}, nil
			}
			return agent.Turn{}, errors.New("API secret-test-value")
		}}
		_, err := summaryDraft(context.Background(), p, conversation, "", redact.New("secret-test-value"))
		if err == nil || strings.Contains(err.Error(), "secret-test-value") || !strings.Contains(err.Error(), "[redacted]") || len(p.requests) != 1 {
			t.Fatalf("provider error: %v (%d calls)", err, len(p.requests))
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &knowledgeProvider{reply: func(agent.Callbacks) (agent.Turn, error) {
		cancel()
		return agent.Turn{Type: "ask_user", Message: "追问"}, nil
	}}
	if _, err := summaryDraft(ctx, p, conversation, "", redact.New()); err == nil || len(p.requests) != 1 {
		t.Fatalf("canceled request retried: %v (%d calls)", err, len(p.requests))
	}
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
	if len(p.requests) != 1 {
		t.Fatal("valid summary was retried")
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
		encoded, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encoded), "会话资料总结助手") || !strings.Contains(string(encoded), "source-start") || !strings.Contains(string(encoded), "source-end") || strings.Contains(string(encoded), "正在本机包装一个真实的 OpenSSH 会话") {
			t.Error("summary lost instructions/source or inherited SSH role")
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
	if err := s.history.Append("profile:"+host.ID, history.Message{ConversationID: c.ID, Role: "user", Content: "source-start" + strings.Repeat("检查服务", 3000) + "source-end"}); err != nil {
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
