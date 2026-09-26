package agent

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// messageStream exposes only the top-level message string of a structured
// response. Incomplete escapes (including UTF-16 pairs) stay buffered; proposals
// and panel actions are not usable until the final response has been validated.
type messageStream struct {
	depth                                             int
	inString, escaped, key, value, wantKey, wantValue bool
	token                                             strings.Builder
	pending                                           string
	field                                             string
	emit                                              func(string)
}

func (s *messageStream) Write(delta string) {
	var output strings.Builder
	for i := 0; i < len(delta); i++ {
		c := delta[i]
		if s.inString {
			if c == '"' && !s.escaped {
				s.inString = false
				if s.key {
					_ = json.Unmarshal([]byte("\""+s.token.String()+"\""), &s.field)
				}
				s.key, s.value = false, false
				continue
			}
			if s.key {
				s.token.WriteByte(c)
			}
			if s.value {
				s.pending += string([]byte{c})
				s.decode(&output)
			}
			if s.escaped {
				s.escaped = false
			} else {
				s.escaped = c == '\\'
			}
			continue
		}
		switch c {
		case '{', '[':
			s.depth++
			if s.depth == 1 {
				s.wantKey = true
			}
		case '}', ']':
			s.depth--
		case ',':
			if s.depth == 1 {
				s.wantKey, s.wantValue = true, false
			}
		case ':':
			if s.depth == 1 {
				s.wantValue = true
			}
		case '"':
			s.inString, s.escaped = true, false
			s.key = s.depth == 1 && s.wantKey
			s.value = s.depth == 1 && s.wantValue && s.field == "message"
			s.wantKey, s.wantValue = false, false
			s.token.Reset()
		}
	}
	if output.Len() > 0 && s.emit != nil {
		s.emit(output.String())
	}
}

func (s *messageStream) decode(output *strings.Builder) {
	for len(s.pending) > 0 {
		if s.pending[0] != '\\' {
			if !utf8.FullRuneInString(s.pending) {
				return
			}
			r, n := utf8.DecodeRuneInString(s.pending)
			output.WriteRune(r)
			s.pending = s.pending[n:]
			continue
		}
		if len(s.pending) < 2 {
			return
		}
		n := 2
		if s.pending[1] == 'u' {
			n = 6
			if len(s.pending) < n {
				return
			}
			// A high surrogate must wait for its low surrogate.
			hex := strings.ToLower(s.pending[2:6])
			if hex >= "d800" && hex <= "dbff" {
				n = 12
			}
			if len(s.pending) < n {
				return
			}
		}
		var value string
		if json.Unmarshal([]byte("\""+s.pending[:n]+"\""), &value) != nil {
			return
		}
		output.WriteString(value)
		s.pending = s.pending[n:]
	}
}
