package app

import (
	"fmt"
	"strings"
)

type rollingBuffer struct {
	limit int
	value string
}

type contextBuffer struct {
	limit int
	total int
	head  []rune
	tail  []rune
}

func newRollingBuffer(limit int) *rollingBuffer {
	return &rollingBuffer{limit: limit}
}

func newContextBuffer(limit int) *contextBuffer {
	if limit <= 0 {
		limit = DefaultAIContextChars
	}
	return &contextBuffer{limit: limit}
}

func (b *rollingBuffer) Append(value string) {
	if value == "" {
		return
	}
	b.value += value
	if len(b.value) > b.limit {
		excess := len(b.value) - b.limit
		b.value = b.value[excess:]
		for len(b.value) > 0 && (b.value[0]&0xc0) == 0x80 {
			b.value = b.value[1:]
		}
	}
}

func (b *rollingBuffer) String() string { return b.value }

func (b *contextBuffer) Append(value string) {
	if value == "" {
		return
	}
	runes := []rune(value)
	b.total += len(runes)
	headLimit := b.limit / 3
	if headLimit < 1 {
		headLimit = 1
	}
	if available := headLimit - len(b.head); available > 0 {
		if available > len(runes) {
			available = len(runes)
		}
		b.head = append(b.head, runes[:available]...)
		runes = runes[available:]
	}
	if len(runes) == 0 {
		return
	}
	tailLimit := b.limit - headLimit
	b.tail = append(b.tail, runes...)
	if excess := len(b.tail) - tailLimit; excess > 0 {
		trimmed := make([]rune, tailLimit)
		copy(trimmed, b.tail[excess:])
		b.tail = trimmed
	}
}

func (b *contextBuffer) ContextString() string {
	if b == nil {
		return ""
	}
	stored := len(b.head) + len(b.tail)
	if b.total <= stored {
		return strings.TrimSpace(string(b.head) + string(b.tail))
	}
	if b.limit < 80 {
		value := append(append([]rune(nil), b.head...), b.tail...)
		if len(value) > b.limit {
			value = value[len(value)-b.limit:]
		}
		return strings.TrimSpace(string(value))
	}
	omitted := b.total - b.limit
	marker := ""
	payload := b.limit
	for range 3 {
		marker = fmt.Sprintf("\n\n... [aissh omitted %d characters because AI context is limited] ...\n\n", omitted)
		payload = b.limit - len([]rune(marker))
		if payload < 2 {
			value := append(append([]rune(nil), b.head...), b.tail...)
			if len(value) > b.limit {
				value = value[len(value)-b.limit:]
			}
			return strings.TrimSpace(string(value))
		}
		omitted = b.total - payload
	}
	headKeep := payload / 3
	if headKeep > len(b.head) {
		headKeep = len(b.head)
	}
	tailKeep := payload - headKeep
	if tailKeep > len(b.tail) {
		tailKeep = len(b.tail)
	}
	return strings.TrimSpace(string(b.head[:headKeep]) + marker + string(b.tail[len(b.tail)-tailKeep:]))
}

func (b *contextBuffer) Reset() {
	b.total = 0
	b.head = b.head[:0]
	b.tail = b.tail[:0]
}

func (a *App) beginCommandOutput(command string) {
	a.terminalResizePending = false
	a.terminalResizeAnchors = nil
	// A mode change or a split PTY read can arrive before any command echo.
	// Do not let the previous prompt complete the new execution in that gap.
	a.outputTail.value = ""
	if a.recentOutput == nil {
		a.recentOutput = newContextBuffer(a.config.AIContextChars)
	} else {
		a.recentOutput.Reset()
	}
	if command = strings.TrimSpace(command); command != "" {
		a.recentOutput.Append("$ " + command + "\n")
	}
}

func appendRuneBackspace(value []rune) []rune {
	if len(value) == 0 {
		return value
	}
	return value[:len(value)-1]
}
