package app

import (
	"bytes"
	"io"
	"os"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

var (
	zmodemStartMarker  = []byte{'*', '*', 0x18, 'B', '0'}
	zmodemFinishMarker = []byte{'*', '*', 0x18, 'B', '0', '8'}
	zmodemCancelMarker = []byte{0x18, 0x18, 0x18, 0x18, 0x18}
)

const zmodemFinishDelay = 750 * time.Millisecond

type zmodemPassthrough struct {
	clientStarted    bool
	handshakeSent    bool
	handshake        []byte
	handshakeTrailer int
	inputTail        []byte
	outputTail       []byte
	finishHeaderSeen bool
	cancelled        bool
	finishSeenAt     time.Time
}

func (a *App) outputWriter() io.Writer {
	if a.screenOut != nil {
		return a.screenOut
	}
	return os.Stdout
}

// processSSHOutput keeps all terminal mutation on the main event loop. This is
// important for ZMODEM: once a transfer starts, protocol bytes must bypass both
// the VT emulator and the normal TUI renderer without racing an in-flight read.
func (a *App) processSSHOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	if a.processInBandMonitorOutput(data) {
		return
	}
	if a.zmodem != nil {
		a.handleZmodemOutput(data)
		return
	}

	combined := append(append([]byte(nil), a.zmodemTail...), data...)
	marker := bytes.Index(combined, zmodemStartMarker)
	if marker < 0 {
		a.processRegularSSHOutput(data)
		a.rememberZmodemTail(data)
		return
	}

	dataMarker := marker - len(a.zmodemTail)
	if dataMarker > 0 {
		a.processRegularSSHOutput(data[:dataMarker])
	}
	start := dataMarker
	if start < 0 {
		// The marker crossed two PTY reads. Re-send its retained prefix verbatim;
		// those bytes were only painted by the VT renderer, never forwarded raw.
		a.enterZmodemPassthrough(combined[marker:])
	} else {
		a.enterZmodemPassthrough(data[start:])
	}
	a.zmodemTail = nil
}

func (a *App) processRegularSSHOutput(data []byte) {
	if len(data) == 0 || a.virtual == nil {
		return
	}
	tokens := a.transformer.TransformTokens(data)
	historyLines := a.writeTerminalOutput(tokens)
	scrolls := len(historyLines)
	a.appendTerminalHistory(historyLines)
	a.scrollInlineBlocks(scrolls)
	a.handleOutput(data)
	a.flushDeferredMonitorInput()
}

func (a *App) rememberZmodemTail(data []byte) {
	keep := len(zmodemStartMarker) - 1
	if keep <= 0 {
		a.zmodemTail = nil
		return
	}
	if len(data) > keep {
		data = data[len(data)-keep:]
	}
	a.zmodemTail = append(a.zmodemTail[:0], data...)
}

func (a *App) enterZmodemPassthrough(data []byte) {
	a.zmodem = &zmodemPassthrough{}
	a.mouseCarry = nil
	a.mouseReporting = false
	a.mouseScrollbarDrag = scrollbarDragState{}
	a.dirty = false
	_, _ = io.WriteString(a.outputWriter(), "\x1b[?2026l"+mouseReportingDisable+"\x1b[0m\x1b[?25h\x1b[?1049l")
	a.handleZmodemOutput(data)
}

func (a *App) handleZmodemOutput(data []byte) {
	state := a.zmodem
	if state == nil || len(data) == 0 {
		return
	}
	state.touchFinishTimer()

	state.outputTail = appendSignalTail(state.outputTail, data)
	if hasZmodemFinish(state.outputTail) {
		state.finishHeaderSeen = true
		// The receiver's ZFIN is the final remote acknowledgement. Keep passing
		// bytes until the short idle window expires so the local helper can reply.
		state.markFinished()
	}
	if bytes.Contains(state.outputTail, zmodemCancelMarker) {
		state.cancelled = true
		state.markFinished()
	}
	if state.finishHeaderSeen && bytes.Contains(state.outputTail, []byte("OO")) {
		state.markFinished()
	}
	if state.cancelled {
		return
	}

	if !state.handshakeSent {
		_, _ = a.outputWriter().Write(data)
		state.handshake = append(state.handshake, data...)
		if end := bytes.IndexByte(state.handshake, '\r'); end >= 0 {
			state.handshakeSent = true
			state.handshakeTrailer = 2 - (len(state.handshake) - end - 1)
			if state.handshakeTrailer < 0 {
				state.handshakeTrailer = 0
			}
			state.handshake = nil
		}
		return
	}

	if !state.clientStarted {
		// rz commonly retries ZRINIT while the file picker is open. Repainting each
		// retry makes terminal integrations reopen the picker forever after cancel.
		// Forward only the LF/XON trailer belonging to the first handshake.
		if state.handshakeTrailer > 0 {
			count := state.handshakeTrailer
			if count > len(data) {
				count = len(data)
			}
			_, _ = a.outputWriter().Write(data[:count])
			state.handshakeTrailer -= count
		}
		return
	}

	_, _ = a.outputWriter().Write(data)
}

func (a *App) handleZmodemInput(data []byte) {
	state := a.zmodem
	if state == nil || len(data) == 0 {
		return
	}
	state.clientStarted = true
	state.touchFinishTimer()
	hadFinishHeader := state.finishHeaderSeen
	state.inputTail = appendSignalTail(state.inputTail, data)
	if hasZmodemFinish(state.inputTail) {
		state.finishHeaderSeen = true
	}
	if bytes.Contains(state.inputTail, zmodemCancelMarker) || bytes.Contains(data, []byte{0x03}) {
		state.cancelled = true
		state.markFinished()
	}
	if hadFinishHeader && bytes.Contains(data, []byte("OO")) {
		state.markFinished()
	}
	if a.sshPTY != nil {
		_, _ = a.sshPTY.Write(data)
	}
}

func appendSignalTail(tail, data []byte) []byte {
	const limit = 96
	tail = append(tail, data...)
	if len(tail) > limit {
		tail = append([]byte(nil), tail[len(tail)-limit:]...)
	}
	return tail
}

func hasZmodemFinish(data []byte) bool {
	return bytes.Contains(data, zmodemFinishMarker)
}

func (state *zmodemPassthrough) markFinished() {
	state.finishSeenAt = time.Now()
}

func (state *zmodemPassthrough) touchFinishTimer() {
	if !state.finishSeenAt.IsZero() {
		state.finishSeenAt = time.Now()
	}
}

func (a *App) checkZmodemCompletion(now time.Time) {
	if a.zmodem == nil || a.zmodem.finishSeenAt.IsZero() || now.Sub(a.zmodem.finishSeenAt) < zmodemFinishDelay {
		return
	}
	a.leaveZmodemPassthrough()
}

func (a *App) leaveZmodemPassthrough() {
	if a.zmodem == nil {
		return
	}
	a.zmodem = nil
	a.zmodemTail = nil
	a.transformer = terminalview.WideTransformer{}
	a.terminalRows = nil
	a.terminalScroll = 0
	a.lineInput = nil
	a.lineCursor = 0
	a.lineAnchored = false
	a.lineReliable = false
	a.terminalKeyCarry = nil
	_, _ = io.WriteString(a.outputWriter(), "\x1b[?1049h\x1b[2J\x1b[H"+mouseReportingDisable)
	a.dirty = true
	// Ask the remote shell for a fresh prompt so the VT model, which intentionally
	// skipped binary transfer traffic, is synchronized again without replaying it.
	if a.sshPTY != nil {
		_, _ = a.sshPTY.Write([]byte{'\r'})
	}
}
