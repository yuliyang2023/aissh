package agent

import (
	"fmt"
	"strings"
)

const DefaultOutputContextChars = 120000

// LimitOutputContext keeps both the beginning (headers and command metadata)
// and the end (errors and summaries) of oversized terminal output. The marker
// makes truncation visible to the model instead of silently dropping data.
func LimitOutputContext(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		limit = DefaultOutputContextChars
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit < 80 {
		return string(runes[len(runes)-limit:])
	}
	omitted := len(runes) - limit
	marker := ""
	payload := limit
	// The omission count changes the marker width, so settle the payload budget
	// before selecting the retained head and tail.
	for range 3 {
		marker = fmt.Sprintf("\n\n... [aissh omitted %d characters because AI context is limited] ...\n\n", omitted)
		payload = limit - len([]rune(marker))
		if payload < 2 {
			return string(runes[len(runes)-limit:])
		}
		omitted = len(runes) - payload
	}
	head := payload / 3
	if head < 1 {
		head = 1
	}
	tail := payload - head
	return string(runes[:head]) + marker + string(runes[len(runes)-tail:])
}
