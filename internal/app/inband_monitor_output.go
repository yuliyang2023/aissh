package app

import "bytes"

// Monitor markers are printable screen text. ConPTY can insert VT controls
// inside them and use cursor positioning in place of line feeds. Keep a map
// back to raw bytes so everything after completion still reaches the prompt
// restorer verbatim, including partially received escape sequences.
type monitorOutputText struct {
	text   []byte
	ends   []int
	parsed int
}

func decodeMonitorOutput(data []byte) monitorOutputText {
	result := monitorOutputText{}
	appendByte := func(value byte, end int) {
		result.text = append(result.text, value)
		result.ends = append(result.ends, end)
	}
	for result.parsed < len(data) {
		index := result.parsed
		value := data[index]
		if value == 0x1b {
			length, complete := monitorPromptControlLength(data[index:])
			if !complete {
				break
			}
			control := data[index : index+length]
			result.parsed += length
			if monitorControlStartsLine(control) && len(result.text) > 0 && result.text[len(result.text)-1] != '\n' {
				appendByte('\n', result.parsed)
			}
			continue
		}
		result.parsed++
		if value >= 0x20 || value == '\r' || value == '\n' || value == '\t' {
			appendByte(value, result.parsed)
		}
	}
	return result
}

func monitorControlStartsLine(control []byte) bool {
	if control[1] == 'D' || control[1] == 'E' {
		return true
	}
	if control[1] != '[' {
		return false
	}
	switch control[len(control)-1] {
	case 'B', 'E', 'd':
		return true
	case 'H', 'f':
		parameters := bytes.Split(control[2:len(control)-1], []byte{';'})
		return len(parameters) == 1 || len(parameters[1]) == 0 || bytes.Equal(parameters[1], []byte("1"))
	}
	return false
}

// Screen wrapping may split a marker across physical rows. Ignore only line
// boundaries while matching; ordinary command echo and OSC titles must never
// impersonate a completed monitor command.
func findMonitorMarker(text, marker []byte) (int, int) {
	for start := 0; start < len(text); start++ {
		if text[start] != marker[0] {
			continue
		}
		position, matched := start, 0
		for position < len(text) && matched < len(marker) {
			if text[position] == '\r' || text[position] == '\n' {
				position++
				continue
			}
			if text[position] != marker[matched] {
				break
			}
			position++
			matched++
		}
		if matched == len(marker) {
			return start, position
		}
	}
	return -1, -1
}

func (output monitorOutputText) suffixStart(characters int) int {
	if len(output.text) == 0 {
		return output.parsed
	}
	start := len(output.text)
	for start > 0 && characters > 0 {
		start--
		if output.text[start] != '\r' && output.text[start] != '\n' {
			characters--
		}
	}
	if start == 0 {
		return 0
	}
	return output.ends[start-1]
}

// Return the raw boundary before a possible split marker. Ordinary short
// samples can stream immediately; only bytes which might be framing wait.
func (output monitorOutputText) beforeMarkerSuffix(marker []byte) int {
	for start := 0; start < len(output.text); start++ {
		if output.text[start] != marker[0] {
			continue
		}
		matched := 0
		for pos := start; pos < len(output.text); pos++ {
			v := output.text[pos]
			if v == '\r' || v == '\n' {
				continue
			}
			if matched >= len(marker) || v != marker[matched] {
				matched = -1
				break
			}
			matched++
		}
		if matched > 0 {
			if start == 0 {
				return 0
			}
			return output.ends[start-1]
		}
	}
	return output.parsed
}
