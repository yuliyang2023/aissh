package agent

import (
	"strings"
	"testing"
)

func TestCatalogCreationUsesItsOwnUntruncatedProtocol(t *testing.T) {
	protocol := strings.Repeat("资料规则", 500) + "protocol-end"
	r := Request{Event: "start", SystemContext: map[string]string{"catalog_assistant": protocol, "conversation_history": "此前用户提供的系统信息"}}
	for _, prompt := range []string{buildCodexPrompt(r, nil, "最新信息"), buildCodexContinuationPrompt(r, "最新信息")} {
		if !strings.Contains(prompt, protocol) || !strings.Contains(prompt, "这里没有 SSH 会话") || !strings.Contains(prompt, "此前用户提供的系统信息") || strings.Contains(prompt, "你是一个 SSH") {
			t.Fatal("creation inherited SSH role or lost protocol")
		}
	}
}
