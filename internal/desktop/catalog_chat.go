package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/history"
	"git.example.com/ai/wrd-ai-cli/internal/knowledgeai"
)

type CatalogChatInput struct {
	Mode           string `json:"mode"`
	ConversationID string `json:"conversationId"`
	Prompt         string `json:"prompt"`
}
type CatalogChatResponse struct {
	Messages []history.Message     `json:"messages"`
	Receipts []knowledgeai.Receipt `json:"receipts"`
	Error    string                `json:"error"`
}

// AskCatalogAssistant belongs to the New dialogs. It never opens an SSH
// session, uses terminal history, or sends data to an SSH target.
func (s *Service) AskCatalogAssistant(in CatalogChatInput) (CatalogChatResponse, error) {
	response := CatalogChatResponse{Messages: []history.Message{}, Receipts: []knowledgeai.Receipt{}}
	if in.Mode != "system" && in.Mode != "knowledge" {
		return response, errors.New("请选择新建外联系统或本地知识")
	}
	if catalog.ValidateID(in.ConversationID) != nil {
		return response, errors.New("创建对话标识无效")
	}
	in.Prompt = strings.TrimSpace(in.Prompt)
	if in.Prompt == "" || len(in.Prompt) > 16000 {
		return response, errors.New("请填写不超过 16 KB 的创建说明")
	}
	if s.history == nil || s.history.Dir == "" {
		return response, errors.New("创建对话需要启用本机历史记录")
	}
	if !s.catalogChatMu.TryLock() {
		return response, errors.New("另一个资料创建请求正在处理，请稍后重试")
	}
	defer s.catalogChatMu.Unlock()
	target := "catalog:" + in.Mode
	conversations, err := s.history.LoadConversations(target)
	if err != nil {
		return response, err
	}
	for _, c := range conversations {
		if c.ID == in.ConversationID {
			response.Messages = c.Messages
			break
		}
	}
	provider, filter, err := s.knowledgeProvider("")
	if err != nil {
		return response, err
	}
	if closer, ok := provider.(io.Closer); ok {
		defer closer.Close()
	}
	appendMessage := func(role, content string) error {
		m := history.Message{ConversationID: in.ConversationID, Role: role, Content: filter.Text(content), Time: time.Now().UTC().Format(time.RFC3339Nano)}
		if err := s.history.Append(target, m); err != nil {
			return err
		}
		response.Messages = append(response.Messages, m)
		return nil
	}
	if err := appendMessage("user", in.Prompt); err != nil {
		return response, err
	}
	var contextText, facts strings.Builder
	for _, m := range response.Messages[max(0, len(response.Messages)-30):] {
		if m.Role == "user" {
			facts.WriteString(m.Content + "\n")
		}
		raw, _ := json.Marshal(map[string]string{"role": m.Role, "content": m.Content})
		contextText.Write(raw)
		contextText.WriteByte('\n')
	}
	assistant := knowledgeai.Assistant{Store: s.store, Mode: in.Mode, Prompt: filter.Text(in.Prompt), UserFacts: facts.String(), ConversationID: in.ConversationID, Filter: filter}
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	defer cancel()
	turn, runErr := assistant.Run(ctx, provider, contextText.String(), func(status string) { s.emit("catalog-create:"+in.ConversationID, filter.Text(status)) })
	response.Receipts = assistant.Receipts
	for _, receipt := range assistant.Receipts {
		label := "已保存："
		if receipt.Status == "unchanged" {
			label = "已存在，无需重复创建："
		}
		if err := appendMessage("system", label+receipt.Detail); err != nil {
			response.Error = "资料已保存，但对话记录写入失败：" + err.Error()
			return response, nil
		}
	}
	if runErr != nil {
		response.Error = filter.Text(runErr.Error())
	} else if turn.Error != "" || turn.Type == "failed" {
		response.Error = filter.Text(turn.Error)
		if response.Error == "" {
			response.Error = filter.Text(turn.Message)
		}
	}
	if response.Error != "" {
		_ = appendMessage("error", response.Error)
	} else if turn.Message != "" {
		if err := appendMessage("assistant", turn.Message); err != nil {
			response.Error = "对话记录写入失败：" + err.Error()
		}
	} else if len(assistant.Receipts) == 0 {
		response.Error = "AI 未返回创建结果，请补充说明后重试"
	}
	return response, nil
}
