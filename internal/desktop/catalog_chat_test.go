package desktop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/history"
)

func TestNewDialogConversationSavesWithDefaultAIAndNoSSH(t *testing.T) {
	s, host, _ := conversationService(t)
	if err := s.history.Append("profile:"+host.ID, history.Message{ConversationID: "ssh-only", Role: "user", Content: "private SSH context"}); err != nil {
		t.Fatal(err)
	}
	step := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(body)
		if strings.Contains(string(encoded), "private SSH context") || !strings.Contains(string(encoded), "这里没有 SSH 会话") {
			t.Error("wrong conversation context")
		}
		if body["model"] != "creation-test" {
			t.Error("wrong default AI")
		}
		response := map[string]any{"type": "finished", "message": "已保存", "proposal": nil, "panelActions": []any{}}
		switch step {
		case 0:
			response["type"] = "ask_user"
			response["message"] = "请提供系统地址。"
		case 1:
			if !strings.Contains(string(encoded), "CMDB") {
				t.Error("lost earlier user fact")
			}
			response["type"] = "command_proposal"
			response["proposal"] = map[string]string{"command": `@knowledge {"action":"save_system","system":{"name":"CMDB","kind":"http","endpoint":"https://cmdb.example.com","description":"资产管理"},"quote":"地址是 https://cmdb.example.com"}`, "explanation": "创建系统", "expectedEffect": "保存到本机", "risk": "low"}
		case 3:
			response["type"] = "command_proposal"
			response["proposal"] = map[string]string{"command": `@knowledge {"action":"upsert_record","title":"资产归属","key":{"IP":"192.0.2.10"},"values":{"部门":"研发部"},"quote":"192.0.2.10 属于研发部"}`, "explanation": "保存记录", "expectedEffect": "建立数据表", "risk": "low"}
		case 4:
			http.Error(w, "model unavailable", http.StatusServiceUnavailable)
			step++
			return
		}
		step++
		raw, _ := json.Marshal(response)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer server.Close()
	if _, err := s.SaveAIProfile(catalog.AIProfile{Name: "默认创建模型", Provider: "model", URL: server.URL, Model: "creation-test"}, true); err != nil {
		t.Fatal(err)
	}
	first, err := s.AskCatalogAssistant(CatalogChatInput{Mode: "system", ConversationID: "new-system", Prompt: "添加 CMDB，类型 HTTP，用于资产管理"})
	if err != nil || first.Error != "" || len(first.Messages) != 2 || len(first.Receipts) != 0 {
		t.Fatalf("question: %+v %v", first, err)
	}
	second, err := s.AskCatalogAssistant(CatalogChatInput{Mode: "system", ConversationID: "new-system", Prompt: "地址是 https://cmdb.example.com"})
	if err != nil || second.Error != "" || len(second.Receipts) != 1 || len(second.Messages) != 5 {
		t.Fatalf("creation: %+v %v", second, err)
	}
	d, _ := s.Snapshot()
	if len(d.Resources) != 1 || d.Resources[0].Name != "CMDB" || d.Resources[0].Grant != "ask" {
		t.Fatalf("system not saved: %+v", d.Resources)
	}
	third, err := s.AskCatalogAssistant(CatalogChatInput{Mode: "knowledge", ConversationID: "new-knowledge", Prompt: "192.0.2.10 属于研发部"})
	if err != nil || third.Error == "" || len(third.Receipts) != 1 {
		t.Fatalf("partial save missing: %+v %v", third, err)
	}
	items, err := catalog.New(s.store.Dir, nil).ListKnowledge()
	if err != nil || len(items) != 1 || len(items[0].Rows) != 1 || items[0].Changes[0].Quote != "192.0.2.10 属于研发部" {
		t.Fatalf("not persisted: %+v %v", items, err)
	}
	if len(s.ListSessions()) != 0 {
		t.Fatal("creation opened SSH")
	}
	conversations, err := s.ListConversations()
	if err != nil || len(conversations) != 1 || conversations[0].ID != "ssh-only" {
		t.Fatalf("creation mixed into SSH history: %+v %v", conversations, err)
	}
	stored, err := s.history.LoadConversations("catalog:system")
	if err != nil || len(stored) != 1 || len(stored[0].Messages) != 5 {
		t.Fatalf("lost creation history: %+v %v", stored, err)
	}
}
