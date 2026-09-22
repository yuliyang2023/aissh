package app

import (
	"io"
	"strings"
	"testing"

	"github.com/hinshun/vt10x"
)

func BenchmarkSSHInputWithAIHistory(b *testing.B) {
	a, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		b.Fatal(err)
	}
	defer a.cancel()
	a.setViewportSize(200, 58)
	a.virtual = vt10x.New(vt10x.WithSize(a.mainCols, a.contentRows))
	a.processSSHOutput([]byte("root@server:~# "))
	a.screenOut = io.Discard
	a.aiExpanded = true
	response := strings.Repeat("服务正常，继续检查日志及网络连接。HTTP 请求已成功返回。\n", 120)
	for index := 0; index < 24; index++ {
		a.chat = append(a.chat, chatMessage{Role: "assistant", Content: response})
		a.inlineHistory = append(a.inlineHistory, &inlineAIState{Prompt: "检查服务", Response: response, Done: true, AnchorX: 15})
	}
	a.render()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		a.processSSHOutput([]byte("\rroot@server:~# x"))
		a.render()
	}
}
