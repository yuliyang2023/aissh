package terminalview

import (
	"strings"
	"testing"
)

func TestWideTransformer(t *testing.T) {
	var transformer WideTransformer
	value := string(transformer.Transform([]byte("中文\x1b[31m红\x1b[0m")))
	if strings.Count(value, string(WidePlaceholder)) != 3 {
		t.Fatalf("transformed = %q", value)
	}
	if !strings.Contains(value, "\x1b[31m") {
		t.Fatalf("escape sequence was changed: %q", value)
	}
}

func TestSplitUTF8(t *testing.T) {
	var transformer WideTransformer
	raw := []byte("中")
	first := transformer.Transform(raw[:1])
	second := transformer.Transform(raw[1:])
	if len(first) != 0 || string(second) != "中"+string(WidePlaceholder) {
		t.Fatalf("first=%q second=%q", first, second)
	}
}

func TestClipPadWide(t *testing.T) {
	if value := ClipPad("中文A", 5); value != "中文A" {
		t.Fatalf("value = %q", value)
	}
	if value := ClipPad("中文A", 4); value != "中文" {
		t.Fatalf("value = %q", value)
	}
}
