package app

import (
	"os"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestClearCommandRemovesInlineAIAndKeepsConversation(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	application.sshPTY = writer
	application.atPrompt = true
	application.aiExpanded = true
	application.inlineAI = &inlineAIState{Prompt: "当前问题", Done: true}
	application.inlineHistory = []*inlineAIState{{Prompt: "旧问题", Done: true}}
	application.chat = append(application.chat, chatMessage{Role: "assistant", Content: "右侧保留"})
	chatCount := len(application.chat)
	application.lineInput = []byte("clear")

	application.submitTerminalLine('\r')

	if application.inlineAI != nil || len(application.inlineHistory) != 0 {
		t.Fatalf("inline output survived clear: current=%#v history=%#v", application.inlineAI, application.inlineHistory)
	}
	if len(application.chat) != chatCount || application.chat[len(application.chat)-1].Content != "右侧保留" {
		t.Fatalf("right conversation was cleared: %#v", application.chat)
	}
	if !application.aiExpanded || application.terminalRows != nil {
		t.Fatalf("expanded=%v terminalRows=%#v", application.aiExpanded, application.terminalRows)
	}
}

func TestRemoteClearSequenceAcrossChunksRemovesInlineAI(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.inlineHistory = []*inlineAIState{{Prompt: "旧问题", Done: true}}
	application.handleOutput([]byte("\x1b[2"))
	if len(application.inlineHistory) != 1 {
		t.Fatal("partial clear sequence cleared output too early")
	}
	application.handleOutput([]byte("J\x1b[H"))
	if len(application.inlineHistory) != 0 {
		t.Fatalf("remote clear did not remove inline output: %#v", application.inlineHistory)
	}
}

func TestCtrlLClearsActiveInlineAIWhileAgentContinues(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	application.sshPTY = writer
	application.agentBusy = true
	application.inlineAI = &inlineAIState{Prompt: "处理中"}
	application.chat = append(application.chat, chatMessage{Role: "user", Content: "处理中"})

	application.handleTerminalInput([]byte{0x0c})

	if application.inlineAI != nil || !application.agentBusy {
		t.Fatalf("inline=%#v busy=%v", application.inlineAI, application.agentBusy)
	}
	buffer := make([]byte, 1)
	if count, err := reader.Read(buffer); err != nil || count != 1 || buffer[0] != 0x0c {
		t.Fatalf("forwarded Ctrl+L count=%d data=%v err=%v", count, buffer[:count], err)
	}
	application.finishAgentTurn(agent.Turn{Type: "finished", Message: "右侧完成"}, nil)
	if application.inlineAI != nil {
		t.Fatalf("cleared inline AI was recreated: %#v", application.inlineAI)
	}
}
