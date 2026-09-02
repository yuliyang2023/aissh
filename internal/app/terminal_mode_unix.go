//go:build !windows

package app

import "golang.org/x/term"

type terminalState struct {
	raw *term.State
}

func makeTerminalRaw(fd int) (*terminalState, error) {
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return &terminalState{raw: state}, nil
}

func restoreTerminal(fd int, state *terminalState) error {
	return term.Restore(fd, state.raw)
}
