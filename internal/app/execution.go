package app

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
)

func newExecutionMarker() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func executionMarkerPrefix(marker string) string {
	return "\x1b]777;aissh-" + marker + ";"
}

// The outer subshell owns the completion trap; the inner one contains all
// proposal side effects, including exit, exec, traps and stdout redirection.
// eval keeps syntax errors inside that inner shell as well. Only printf emits
// the actual OSC bytes, so terminal echo cannot impersonate command completion.
func isolateApprovedCommand(command, marker string) string {
	return isolateApprovedCommandWithLineEnding(command, marker, approvedCommandLineEnding())
}

func isolateApprovedCommandWithLineEnding(command, marker, lineEnding string) string {
	command = strings.ReplaceAll(command, "\r\n", "\n")
	command = strings.ReplaceAll(command, "\r", "\n")
	completion := "printf '\\033]777;aissh-" + marker + ";%s\\007' \"$?\""
	wrapper := "(\ntrap " + quotePOSIXShell(completion) + " 0\ntrap 'exit 130' 2\n(\ntrap - 0 2\neval " + quotePOSIXShell(command) + "\n)\n)"
	return strings.ReplaceAll(wrapper, "\n", lineEnding) + "\r"
}

// consume removes only this execution's complete OSC marker. It retains partial
// markers across PTY reads while passing ordinary output through immediately.
// Output after the marker belongs to the next shell prompt, not this command.
func (state *executionState) consume(data []byte) (output, after []byte, exitCode *int) {
	buffer := append(state.Pending, data...)
	state.Pending = nil
	prefix := []byte(executionMarkerPrefix(state.Marker))
	for len(buffer) > 0 {
		index := bytes.Index(buffer, prefix)
		if index < 0 {
			keep := len(prefix) - 1
			if keep > len(buffer) {
				keep = len(buffer)
			}
			for keep > 0 && !bytes.Equal(buffer[len(buffer)-keep:], prefix[:keep]) {
				keep--
			}
			output = append(output, buffer[:len(buffer)-keep]...)
			state.Pending = append([]byte(nil), buffer[len(buffer)-keep:]...)
			return output, nil, nil
		}
		output = append(output, buffer[:index]...)
		buffer = buffer[index:]
		status := buffer[len(prefix):]
		digits := 0
		for digits < len(status) && digits < 3 && status[digits] >= '0' && status[digits] <= '9' {
			digits++
		}
		if digits == len(status) {
			state.Pending = append([]byte(nil), buffer...)
			return output, nil, nil
		}
		if digits > 0 && status[digits] == '\a' {
			code, err := strconv.Atoi(string(status[:digits]))
			if err == nil && code <= 255 {
				return output, status[digits+1:], &code
			}
		}
		// A malformed marker is ordinary output. Advance so it cannot stall
		// collection or grow the pending buffer without bound.
		output = append(output, buffer[0])
		buffer = buffer[1:]
	}
	return output, nil, nil
}
