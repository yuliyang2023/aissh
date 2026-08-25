package app

import "strings"

type rollingBuffer struct {
	limit int
	value string
}

func newRollingBuffer(limit int) *rollingBuffer {
	return &rollingBuffer{limit: limit}
}

func (b *rollingBuffer) Append(value string) {
	if value == "" {
		return
	}
	b.value += value
	if len(b.value) > b.limit {
		b.value = b.value[len(b.value)-b.limit:]
		for len(b.value) > 0 && (b.value[0]&0xc0) == 0x80 {
			b.value = b.value[1:]
		}
	}
}

func (b *rollingBuffer) String() string { return strings.TrimSpace(b.value) }

func appendRuneBackspace(value []rune) []rune {
	if len(value) == 0 {
		return value
	}
	return value[:len(value)-1]
}
