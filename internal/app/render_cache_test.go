package app

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
)

func TestConversationCacheFollowsStreamingHistoryAndProposalChanges(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	proposal := &agent.Proposal{Command: "uptime", Explanation: "检查服务", Risk: "read_only"}
	a.chat = []chatMessage{{Role: "assistant_stream", Content: "第一段"}, {Role: "proposal", Proposal: proposal}}
	check := func(width int) {
		t.Helper()
		if got, want := a.aiDisplayLines(width), a.buildAIConversationLines(width); !reflect.DeepEqual(got, want) {
			t.Fatal("conversation cache left stale text, wrapping or risk styling")
		}
	}
	check(60)
	a.chat[0].Content += "，继续分析"
	check(60)
	proposal.Risk = "high"
	a.proposal = proposal
	check(60)
	a.chat[1].CommandExpanded = true
	proposal.Command = strings.Repeat("echo hello; ", 20)
	check(60)
	check(30)
	a.chat = []chatMessage{{Role: "user", Content: "另一个会话"}, {Role: "assistant", Content: "新的回答"}}
	a.proposal = nil
	a.agentThought = "正在分析"
	check(30)
}

func TestCompletedInlineAnswerCacheFollowsContentAndResize(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	state := &inlineAIState{Done: true, Response: strings.Repeat("检查结果正常。", 20)}
	for _, width := range []int{60, 30, 90} {
		for _, available := range []int{5, 15} {
			if got, want := a.inlineConversationLinesFor(state, width, available), a.buildInlineAnswerLines(state, width, available); !reflect.DeepEqual(got, want) {
				t.Fatal("inline answer retained old wrapping after resizing")
			}
		}
	}
	state.Error, state.Response = "failed", "请求失败"
	if got := strings.Join(a.inlineConversationLinesFor(state, 90, 15), "\n"); !strings.Contains(got, "AI 错误: 请求失败") {
		t.Fatalf("inline error did not replace cached answer: %q", got)
	}
}

func TestSSHInputDoesNotRepaintUnchangedSidebar(t *testing.T) {
	a, _ := newMonitorTestApp(t)
	var screen bytes.Buffer
	a.screenOut = &screen
	a.aiExpanded = true
	a.chat = []chatMessage{{Role: "assistant", Content: "保留在右侧的回答"}}
	a.render()
	if !strings.Contains(screen.String(), "保留在右侧的回答") {
		t.Fatal("initial sidebar was not painted")
	}
	initialBytes := screen.Len()
	screen.Reset()
	a.processSSHOutput([]byte("ls"))
	a.render()
	if strings.Contains(screen.String(), "保留在右侧的回答") || !strings.Contains(screen.String(), "ls") {
		t.Fatal("SSH echo redrew the conversation or lost input echo")
	}
	if screen.Len() >= initialBytes/2 {
		t.Fatalf("SSH input still emitted most of the screen: initial=%d input=%d", initialBytes, screen.Len())
	}
	a.toggleSidebar()
	a.render()
	screen.Reset()
	a.toggleSidebar()
	a.render()
	if !strings.Contains(screen.String(), "保留在右侧的回答") {
		t.Fatal("sidebar was not restored after expanding")
	}
}
