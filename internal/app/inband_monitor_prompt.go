package app

// The hidden query advances the remote terminal, but the local SSH pane keeps
// its original prompt row. Consume the monitor's trailing line breaks and
// cursor moves even when they arrive after the completion marker in later
// reads. Prompt text, styling and terminal modes still reach the local VT.
type monitorPromptState struct {
	pending []byte
	styles  []byte
}

func (state *monitorPromptState) consume(data []byte) ([]byte, bool) {
	state.pending = append(state.pending, data...)
	for len(state.pending) > 0 {
		value := state.pending[0]
		switch {
		case value == 0x1b:
			length, complete := monitorPromptControlLength(state.pending)
			if !complete {
				return nil, false
			}
			control := state.pending[:length]
			if keepMonitorPromptControl(control) {
				state.styles = append(state.styles, control...)
			}
			state.pending = state.pending[length:]
		case value < 0x20:
			// Keep non-layout controls such as BEL and character-set shifts.
			switch value {
			case '\r', '\n', '\v', '\f', '\b':
			default:
				state.styles = append(state.styles, value)
			}
			state.pending = state.pending[1:]
		default:
			visible := append([]byte(nil), inBandMonitorScreenRestore...)
			visible = append(visible, state.styles...)
			visible = append(visible, state.pending...)
			return visible, true
		}
	}
	return nil, false
}

// Unlike keyboard escape parsing, prompt output includes OSC titles and
// hyperlinks. Treat each control as a unit so a split sequence cannot leak
// its parameters into the terminal as text.
func monitorPromptControlLength(data []byte) (int, bool) {
	if len(data) < 2 {
		return 0, false
	}
	switch data[1] {
	case '[':
		for index := 2; index < len(data); index++ {
			if data[index] >= 0x40 && data[index] <= 0x7e {
				return index + 1, true
			}
		}
	case ']', 'P', '_', '^', 'X':
		for index := 2; index < len(data); index++ {
			if data[1] == ']' && data[index] == 0x07 {
				return index + 1, true
			}
			if data[index] == 0x1b && index+1 < len(data) && data[index+1] == '\\' {
				return index + 2, true
			}
		}
	default:
		for index := 1; index < len(data); index++ {
			if data[index] < 0x20 || data[index] > 0x2f {
				return index + 1, true
			}
		}
	}
	return 0, false
}

func keepMonitorPromptControl(control []byte) bool {
	if control[1] == '[' {
		switch control[len(control)-1] {
		case 'm', 'h', 'l', 'q': // SGR, terminal modes, cursor style
			return true
		default:
			return false
		}
	}
	switch control[1] {
	case '7', '8', 'D', 'E', 'M', 'c': // cursor save/restore, movement, reset
		return false
	default:
		return true
	}
}
