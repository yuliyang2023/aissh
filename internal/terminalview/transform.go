package terminalview

import (
	"bytes"
	"unicode/utf8"
)

// WidePlaceholder occupies the second virtual cell of a wide rune. vt10x is a
// one-cell-per-rune emulator, so aissh inserts this marker outside escape
// sequences and omits it while painting the physical terminal.
const WidePlaceholder = '\ue000'

type transformMode uint8

const (
	modeText transformMode = iota
	modeEscape
	modeCSI
	modeOSC
	modeOSCEscape
)

type WideTransformer struct {
	mode  transformMode
	carry []byte
}

type OutputTokenKind uint8

const (
	OutputControl OutputTokenKind = iota
	OutputText
	OutputLineFeed
)

type OutputToken struct {
	Data []byte
	Kind OutputTokenKind
}

func (t *WideTransformer) Transform(input []byte) []byte {
	tokens := t.TransformTokens(input)
	var output bytes.Buffer
	for _, token := range tokens {
		output.Write(token.Data)
	}
	return output.Bytes()
}

// TransformTokens preserves the terminal byte stream while distinguishing
// printable screen cells from control sequences. Callers that emulate the
// terminal a token at a time can therefore observe soft-wrap scrolling without
// mistaking bytes inside CSI/OSC sequences for visible text.
func (t *WideTransformer) TransformTokens(input []byte) []OutputToken {
	data := append(append([]byte(nil), t.carry...), input...)
	t.carry = t.carry[:0]
	tokens := make([]OutputToken, 0, len(data))
	appendToken := func(value []byte, kind OutputTokenKind) {
		tokens = append(tokens, OutputToken{Data: value, Kind: kind})
	}
	for index := 0; index < len(data); {
		value := data[index]
		switch t.mode {
		case modeEscape:
			appendToken(data[index:index+1], OutputControl)
			index++
			switch value {
			case '[':
				t.mode = modeCSI
			case ']', 'P', '_', '^', 'k':
				t.mode = modeOSC
			default:
				t.mode = modeText
			}
			continue
		case modeCSI:
			appendToken(data[index:index+1], OutputControl)
			index++
			if value >= 0x40 && value <= 0x7e {
				t.mode = modeText
			}
			continue
		case modeOSC:
			appendToken(data[index:index+1], OutputControl)
			index++
			if value == 0x07 {
				t.mode = modeText
			} else if value == 0x1b {
				t.mode = modeOSCEscape
			}
			continue
		case modeOSCEscape:
			appendToken(data[index:index+1], OutputControl)
			index++
			if value == '\\' {
				t.mode = modeText
			} else {
				t.mode = modeOSC
			}
			continue
		}

		if value == 0x1b {
			appendToken(data[index:index+1], OutputControl)
			index++
			t.mode = modeEscape
			continue
		}
		if value < utf8.RuneSelf {
			kind := OutputText
			if value < 0x20 || value == 0x7f {
				kind = OutputControl
				if value == '\n' {
					kind = OutputLineFeed
				}
			}
			appendToken(data[index:index+1], kind)
			index++
			continue
		}
		if !utf8.FullRune(data[index:]) {
			t.carry = append(t.carry, data[index:]...)
			break
		}
		r, size := utf8.DecodeRune(data[index:])
		if r == utf8.RuneError && size == 1 {
			appendToken(data[index:index+1], OutputControl)
			index++
			continue
		}
		appendToken(data[index:index+size], OutputText)
		if CellWidth(r) == 2 {
			appendToken([]byte(string(WidePlaceholder)), OutputText)
		}
		index += size
	}
	return tokens
}

func CellWidth(r rune) int {
	if r == 0 || r == WidePlaceholder {
		return 0
	}
	if r < 0x1100 {
		return 1
	}
	if r <= 0x115f || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff) ||
		(r >= 0x20000 && r <= 0x3fffd) {
		return 2
	}
	return 1
}
