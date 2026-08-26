package app

import (
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type aiEditorLayout struct {
	Lines     []string
	CursorRow int
	CursorCol int
}

func (a *App) clampAIEditorCursor() {
	if a.aiEditorCursor < 0 {
		a.aiEditorCursor = 0
	}
	if a.aiEditorCursor > len(a.editor) {
		a.aiEditorCursor = len(a.editor)
	}
}

func (a *App) insertAIEditorRune(value rune) {
	a.clampAIEditorCursor()
	a.editor = append(a.editor, 0)
	copy(a.editor[a.aiEditorCursor+1:], a.editor[a.aiEditorCursor:])
	a.editor[a.aiEditorCursor] = value
	a.aiEditorCursor++
}

func (a *App) backspaceAIEditorRune() {
	a.clampAIEditorCursor()
	if a.aiEditorCursor == 0 {
		return
	}
	copy(a.editor[a.aiEditorCursor-1:], a.editor[a.aiEditorCursor:])
	a.editor = a.editor[:len(a.editor)-1]
	a.aiEditorCursor--
}

func (a *App) deleteAIEditorRune() {
	a.clampAIEditorCursor()
	if a.aiEditorCursor >= len(a.editor) {
		return
	}
	copy(a.editor[a.aiEditorCursor:], a.editor[a.aiEditorCursor+1:])
	a.editor = a.editor[:len(a.editor)-1]
}

func (a *App) moveAIEditorCursor(delta int) {
	a.aiEditorCursor += delta
	a.clampAIEditorCursor()
}

func (a *App) moveAIEditorHome() {
	a.clampAIEditorCursor()
	for a.aiEditorCursor > 0 && a.editor[a.aiEditorCursor-1] != '\n' {
		a.aiEditorCursor--
	}
}

func (a *App) moveAIEditorEnd() {
	a.clampAIEditorCursor()
	for a.aiEditorCursor < len(a.editor) && a.editor[a.aiEditorCursor] != '\n' {
		a.aiEditorCursor++
	}
}

func (a *App) moveAIEditorVertical(delta int) {
	a.clampAIEditorCursor()
	lineStart := a.aiEditorCursor
	for lineStart > 0 && a.editor[lineStart-1] != '\n' {
		lineStart--
	}
	column := a.aiEditorCursor - lineStart
	if delta < 0 {
		if lineStart == 0 {
			return
		}
		previousEnd := lineStart - 1
		previousStart := previousEnd
		for previousStart > 0 && a.editor[previousStart-1] != '\n' {
			previousStart--
		}
		length := previousEnd - previousStart
		if column > length {
			column = length
		}
		a.aiEditorCursor = previousStart + column
		return
	}
	lineEnd := a.aiEditorCursor
	for lineEnd < len(a.editor) && a.editor[lineEnd] != '\n' {
		lineEnd++
	}
	if lineEnd == len(a.editor) {
		return
	}
	nextStart := lineEnd + 1
	nextEnd := nextStart
	for nextEnd < len(a.editor) && a.editor[nextEnd] != '\n' {
		nextEnd++
	}
	length := nextEnd - nextStart
	if column > length {
		column = length
	}
	a.aiEditorCursor = nextStart + column
}

// consumeAIEditorEscape handles complete terminal key sequences. A lone Esc is
// deliberately left unhandled so the caller can return focus to SSH.
func (a *App) consumeAIEditorEscape(data []byte) (int, bool) {
	if len(data) < 2 || data[0] != 0x1b {
		return 0, false
	}
	if data[1] == '\r' || data[1] == '\n' { // Alt+Enter on several terminals.
		a.insertAIEditorRune('\n')
		return 2, true
	}
	if data[1] == 'O' {
		if len(data) < 3 {
			return 0, false
		}
		switch data[2] {
		case 'A':
			a.moveAIEditorVertical(-1)
		case 'B':
			a.moveAIEditorVertical(1)
		case 'C':
			a.moveAIEditorCursor(1)
		case 'D':
			a.moveAIEditorCursor(-1)
		case 'H':
			a.moveAIEditorHome()
		case 'F':
			a.moveAIEditorEnd()
		}
		return 3, true
	}
	if data[1] != '[' {
		return 2, true
	}
	end := -1
	for index := 2; index < len(data); index++ {
		if data[index] >= 0x40 && data[index] <= 0x7e {
			end = index
			break
		}
	}
	if end < 0 {
		return 0, false
	}
	parameters := string(data[2:end])
	switch data[end] {
	case 'A':
		a.moveAIEditorVertical(-1)
	case 'B':
		a.moveAIEditorVertical(1)
	case 'C':
		a.moveAIEditorCursor(1)
	case 'D':
		a.moveAIEditorCursor(-1)
	case 'H':
		a.moveAIEditorHome()
	case 'F':
		a.moveAIEditorEnd()
	case 'u':
		if strings.HasPrefix(parameters, "13;") {
			a.insertAIEditorRune('\n')
		}
	case '~':
		switch parameters {
		case "1", "7":
			a.moveAIEditorHome()
		case "4", "8":
			a.moveAIEditorEnd()
		case "3":
			a.deleteAIEditorRune()
		case "13;2", "27;2;13":
			a.insertAIEditorRune('\n')
		case "200":
			a.aiPasteMode = true
		case "201":
			a.aiPasteMode = false
		}
	}
	return end + 1, true
}

func (a *App) buildAIEditorLayout(width, bodyRows int) aiEditorLayout {
	a.clampAIEditorCursor()
	if bodyRows <= 0 {
		return aiEditorLayout{}
	}
	textWidthLimit := width - 4
	if textWidthLimit < 1 {
		textWidthLimit = 1
	}
	var physical []string
	var current strings.Builder
	currentWidth := 0
	cursorRow, cursorCol := 0, 0
	cursorSet := false
	flush := func() {
		physical = append(physical, current.String())
		current.Reset()
		currentWidth = 0
	}
	for index, value := range a.editor {
		cellWidth := terminalview.CellWidth(value)
		if value != '\n' && currentWidth > 0 && currentWidth+cellWidth > textWidthLimit {
			flush()
		}
		if index == a.aiEditorCursor {
			cursorRow = len(physical)
			cursorCol = currentWidth
			cursorSet = true
		}
		if value == '\n' {
			flush()
			continue
		}
		current.WriteRune(value)
		currentWidth += cellWidth
	}
	if !cursorSet {
		cursorRow = len(physical)
		cursorCol = currentWidth
	}
	flush()

	start := 0
	if len(physical) > bodyRows {
		start = cursorRow - bodyRows + 1
		if start < 0 {
			start = 0
		}
		maximum := len(physical) - bodyRows
		if start > maximum {
			start = maximum
		}
	}
	end := start + bodyRows
	if end > len(physical) {
		end = len(physical)
	}
	lines := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		prefix := " ↳ "
		if index == 0 {
			prefix = " › "
		}
		lines = append(lines, prefix+physical[index])
	}
	return aiEditorLayout{Lines: lines, CursorRow: cursorRow - start, CursorCol: cursorCol}
}

func aiInputBodyRows(height int) int {
	if height < 3 {
		return 0
	}
	if height >= 9 {
		return 3
	}
	if height >= 5 {
		return 2
	}
	return 1
}

func (a *App) aiEditorCursorPosition(top, left, width, height int) (int, int, bool) {
	bodyRows := aiInputBodyRows(height)
	if bodyRows == 0 {
		return 0, 0, false
	}
	conversationHeight := aiConversationViewport(height, true)
	inputTop := top + 1 + conversationHeight
	layout := a.buildAIEditorLayout(width, bodyRows)
	row := inputTop + 1 + layout.CursorRow
	col := left + 3 + layout.CursorCol
	if row > top+height-1 {
		row = top + height - 1
	}
	if col >= left+width {
		col = left + width - 1
	}
	return row, col, true
}
