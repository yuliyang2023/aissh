package app

import (
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

func (a *App) appendTerminalHistory(lines []terminalview.Line) {
	if len(lines) == 0 {
		return
	}
	if a.terminalScroll > 0 {
		a.terminalScroll += len(lines)
	}
	a.terminalHistory = append(a.terminalHistory, lines...)
	limit := a.config.ScrollbackLines
	if limit <= 0 {
		limit = DefaultScrollbackLines
	}
	if excess := len(a.terminalHistory) - limit; excess > 0 {
		trimmed := make([]terminalview.Line, limit)
		copy(trimmed, a.terminalHistory[excess:])
		a.terminalHistory = trimmed
	}
	if a.terminalScroll > len(a.terminalHistory) {
		a.terminalScroll = len(a.terminalHistory)
	}
}

func (a *App) scrollTerminal(up bool) {
	if len(a.terminalHistory) == 0 {
		return
	}
	if up {
		a.terminalScroll += 3
		if a.terminalScroll > len(a.terminalHistory) {
			a.terminalScroll = len(a.terminalHistory)
		}
	} else {
		a.terminalScroll -= 3
		if a.terminalScroll < 0 {
			a.terminalScroll = 0
		}
	}
	a.focus = focusTerminal
}

func (a *App) jumpTerminalScroll(row int) {
	if len(a.terminalHistory) == 0 || a.contentRows <= 1 {
		return
	}
	relative := row - 2
	if relative < 0 {
		relative = 0
	}
	if relative >= a.contentRows {
		relative = a.contentRows - 1
	}
	start := len(a.terminalHistory) * relative / (a.contentRows - 1)
	a.terminalScroll = len(a.terminalHistory) - start
	a.focus = focusTerminal
}

func (a *App) renderTerminalScrollback() (string, int, int) {
	live := terminalview.Snapshot(a.virtual, a.contentRows, a.mainCols)
	total := len(a.terminalHistory) + len(live)
	end := total - a.terminalScroll
	start := end - a.contentRows
	rows := make(map[int]string, a.contentRows)
	for index := 0; index < a.contentRows; index++ {
		logical := start + index
		var line terminalview.Line
		switch {
		case logical >= 0 && logical < len(a.terminalHistory):
			line = a.terminalHistory[logical]
		case logical >= len(a.terminalHistory) && logical < total:
			line = live[logical-len(a.terminalHistory)]
		}
		row := 2 + index
		rows[row] = terminalview.RenderLine(line, row, 1, a.mainCols)
	}
	a.addScrollbarToRows(rows, 2, a.mainCols, a.contentRows, total, a.contentRows, a.terminalScroll)
	return a.renderTerminalRowChanges(rows), 2, 1
}

func (a *App) renderTerminalRowChanges(rows map[int]string) string {
	var output strings.Builder
	for row := 2; row < a.screenRows(); row++ {
		if a.terminalRows == nil || a.terminalRows[row] != rows[row] {
			output.WriteString(rows[row])
		}
	}
	a.terminalRows = rows
	return output.String()
}

func (a *App) addScrollbarToRows(rows map[int]string, top, col, height, total, viewport, offset int) {
	if total <= viewport || height <= 0 {
		return
	}
	thumbTop, thumbSize := scrollbarThumb(height, total, viewport, offset)
	for index := 0; index < height; index++ {
		glyph := "│"
		style := "\x1b[38;5;238m"
		if index >= thumbTop && index < thumbTop+thumbSize {
			glyph = "█"
			style = "\x1b[38;5;45m"
		}
		row := top + index
		rows[row] += positionedLine(row, col, glyph, 1, style)
	}
}

func renderScrollbar(top, col, height, total, viewport, offset int) string {
	if total <= viewport || height <= 0 {
		return ""
	}
	thumbTop, thumbSize := scrollbarThumb(height, total, viewport, offset)
	var output strings.Builder
	for index := 0; index < height; index++ {
		glyph := "│"
		style := "\x1b[38;5;238m"
		if index >= thumbTop && index < thumbTop+thumbSize {
			glyph = "█"
			style = "\x1b[38;5;45m"
		}
		output.WriteString(positionedLine(top+index, col, glyph, 1, style))
	}
	return output.String()
}

func scrollbarThumb(height, total, viewport, offset int) (int, int) {
	thumbSize := height * viewport / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}
	maximumStart := total - viewport
	if maximumStart <= 0 {
		return 0, thumbSize
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maximumStart {
		offset = maximumStart
	}
	start := maximumStart - offset
	thumbTop := (height - thumbSize) * start / maximumStart
	return thumbTop, thumbSize
}
