package app

import (
	"strings"
	"unicode/utf8"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type terminalRowPlacement struct {
	SourceRow  int
	LogicalRow int
}

type inlineBlockLayout struct {
	State      *inlineAIState
	LogicalTop int
	Top        int
	Left       int
	Width      int
	Height     int
	Lines      []string
}

type terminalLayoutModel struct {
	Rows          []terminalRowPlacement
	Blocks        []inlineBlockLayout
	CursorLogical int
	Viewport      int
}

type capturedLineLayout struct {
	Lines     []string
	CursorRow int
	CursorCol int
}

func (a *App) renderTerminalWorkspace() (string, int, int) {
	if a.virtual == nil || a.contentRows <= 0 || a.mainCols <= 0 {
		return "", 2, 1
	}
	if a.terminalScroll > 0 {
		return a.renderTerminalScrollback()
	}
	model := a.buildTerminalLayout()
	captured := capturedLineLayout{}
	captureVisible := a.aiLineCapture && a.focus == focusTerminal
	if captureVisible {
		cursor := a.virtual.Cursor()
		captured = buildCapturedLineLayout(a.lineInput, a.lineCursor, cursor.X, a.mainCols)
		visualCursorLogical := model.CursorLogical + captured.CursorRow
		if visualCursorLogical >= a.contentRows {
			model.Viewport = visualCursorLogical - (a.contentRows - 1)
		}
		for index := range model.Blocks {
			model.Blocks[index].Top = 2 + model.Blocks[index].LogicalTop - model.Viewport
		}
	}
	rows := make(map[int]string, a.contentRows)
	for row := 2; row < a.screenRows(); row++ {
		rows[row] = positionedLine(row, 1, "", a.mainCols, "\x1b[0m")
	}
	for _, placement := range model.Rows {
		target := 2 + placement.LogicalRow - model.Viewport
		if target < 2 || target >= a.screenRows() {
			continue
		}
		rows[target] = terminalview.RenderVTLine(a.virtual, placement.SourceRow, target, 1, a.mainCols)
	}
	for _, layout := range model.Blocks {
		state := layout.State
		if state == nil || layout.Width <= 0 {
			continue
		}
		if layout.Top >= 2 && layout.Top < a.screenRows() {
			rows[layout.Top] += positionedLine(layout.Top, layout.Left, state.Prompt, layout.Width, "\x1b[38;5;213m")
		}
		for index, line := range layout.Lines {
			row := layout.Top + 1 + index
			if row < 2 || row >= a.screenRows() {
				continue
			}
			rows[row] = positionedLine(row, 1, "", a.mainCols, "\x1b[0m") +
				positionedLine(row, layout.Left, line, layout.Width, a.inlineLineStyle(state, line))
		}
	}
	a.addScrollbarToRows(rows, 2, a.terminalScrollbarCol(), a.contentRows, len(a.terminalHistory)+a.contentRows, a.contentRows, 0)
	output := a.renderTerminalRowChanges(rows)
	cursor := a.virtual.Cursor()
	cursorRow := 2 + model.CursorLogical - model.Viewport
	cursorCol := 1 + cursor.X
	if captureVisible {
		for index, line := range captured.Lines {
			row := cursorRow + index
			if row < 2 || row >= a.screenRows() {
				continue
			}
			col, width := 1, a.mainCols
			if index == 0 {
				col = 1 + cursor.X
				width = a.mainCols - cursor.X
			}
			output += positionedLine(row, col, line, width, "\x1b[38;5;213m")
			// The captured line is an overlay rather than remote VT state. Make
			// the underlying rows render again on the next frame so shrinking or
			// moving the input cannot leave stale characters behind.
			delete(a.terminalRows, row)
		}
		cursorRow += captured.CursorRow
		cursorCol = captured.CursorCol
	}
	if cursorRow < 2 {
		cursorRow = 2
	}
	if cursorRow >= a.screenRows() {
		cursorRow = a.screenRows() - 1
	}
	if cursorCol < 1 {
		cursorCol = 1
	}
	if cursorCol > a.mainCols {
		cursorCol = a.mainCols
	}
	return output, cursorRow, cursorCol
}

func buildCapturedLineLayout(value []byte, cursor, startX, width int) capturedLineLayout {
	if width <= 0 {
		return capturedLineLayout{}
	}
	startX = normalizedAnchorX(startX, width)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	for cursor > 0 && cursor < len(value) && !utf8.RuneStart(value[cursor]) {
		cursor--
	}

	layout := capturedLineLayout{}
	var line strings.Builder
	lineStartX := startX
	lineWidth := width - lineStartX
	used := 0
	cursorSet := false
	setCursor := func() {
		layout.CursorRow = len(layout.Lines)
		layout.CursorCol = lineStartX + used + 1
		if layout.CursorCol > width {
			layout.CursorCol = width
		}
		cursorSet = true
	}
	flush := func() {
		layout.Lines = append(layout.Lines, line.String())
		line.Reset()
		lineStartX = 0
		lineWidth = width
		used = 0
	}

	for index, r := range string(value) {
		if r == '\r' {
			if index == cursor {
				setCursor()
			}
			continue
		}
		if r == '\n' {
			if index == cursor {
				setCursor()
			}
			flush()
			continue
		}
		cellWidth := terminalview.CellWidth(r)
		if used+cellWidth > lineWidth && (used > 0 || lineStartX > 0) {
			flush()
		}
		if index == cursor {
			setCursor()
		}
		line.WriteRune(r)
		used += cellWidth
	}
	if !cursorSet {
		setCursor()
	}
	flush()
	return layout
}

func (a *App) buildTerminalLayout() terminalLayoutModel {
	blocks := a.inlineBlocks()
	model := terminalLayoutModel{}
	logicalRow := 0
	cursorY := 0
	if a.virtual != nil {
		cursorY = a.virtual.Cursor().Y
	}
	if cursorY < 0 {
		cursorY = 0
	}
	if cursorY >= a.contentRows {
		cursorY = a.contentRows - 1
	}
	for sourceRow := 0; sourceRow < a.contentRows; sourceRow++ {
		model.Rows = append(model.Rows, terminalRowPlacement{SourceRow: sourceRow, LogicalRow: logicalRow})
		for _, state := range blocks {
			if normalizedAnchorY(state.AnchorY, a.contentRows) != sourceRow {
				continue
			}
			anchorX := normalizedAnchorX(state.AnchorX, a.mainCols)
			width := a.mainCols - anchorX
			available := a.contentRows - 1
			if available < 1 {
				available = 1
			}
			lines := a.inlineConversationLinesFor(state, width, available)
			height := len(lines)
			if height < 1 {
				height = 1
			}
			model.Blocks = append(model.Blocks, inlineBlockLayout{
				State:      state,
				LogicalTop: logicalRow,
				Left:       1 + anchorX,
				Width:      width,
				Height:     height,
				Lines:      lines,
			})
			logicalRow += height + 1
			model.Rows = append(model.Rows, terminalRowPlacement{SourceRow: sourceRow, LogicalRow: logicalRow})
		}
		if sourceRow == cursorY {
			model.CursorLogical = logicalRow
		}
		logicalRow++
	}
	if model.CursorLogical >= a.contentRows {
		model.Viewport = model.CursorLogical - (a.contentRows - 1)
	}
	for index := range model.Blocks {
		model.Blocks[index].Top = 2 + model.Blocks[index].LogicalTop - model.Viewport
	}
	return model
}

func (a *App) inlineBlocks() []*inlineAIState {
	blocks := make([]*inlineAIState, 0, len(a.inlineHistory)+1)
	blocks = append(blocks, a.inlineHistory...)
	if a.inlineAI != nil {
		blocks = append(blocks, a.inlineAI)
	}
	return blocks
}

// Command completion and the next shell prompt can arrive in separate reads.
// Keep the review beside that prompt even if analysis started in the gap.
func (a *App) resolveInlineExecutionAnchor() {
	state := a.inlineAI
	if state == nil || !state.AwaitingPrompt || a.virtual == nil {
		return
	}
	cursor := a.virtual.Cursor()
	state.AnchorX, state.AnchorY = cursor.X, cursor.Y
	state.AwaitingPrompt = false
	a.terminalRows = nil
}

func (a *App) scrollInlineBlocks(lines int) {
	if lines <= 0 {
		return
	}
	kept := a.inlineHistory[:0]
	for _, state := range a.inlineHistory {
		state.AnchorY -= lines
		if state.AnchorY >= 0 {
			kept = append(kept, state)
		}
	}
	a.inlineHistory = kept
	if a.inlineAI != nil {
		a.inlineAI.AnchorY -= lines
		if a.inlineAI.AnchorY < 0 && !a.agentBusy && a.proposal == nil {
			a.inlineAI = nil
		}
	}
}

func (a *App) inlineLayoutFor(state *inlineAIState) (inlineBlockLayout, bool) {
	if state == nil {
		return inlineBlockLayout{}, false
	}
	for _, layout := range a.buildTerminalLayout().Blocks {
		if layout.State == state {
			return layout, true
		}
	}
	return inlineBlockLayout{}, false
}

func (a *App) renderInlineLayout(layout inlineBlockLayout) string {
	state := layout.State
	if state == nil || layout.Width <= 0 {
		return ""
	}
	var output strings.Builder
	if layout.Top >= 2 && layout.Top < a.screenRows() {
		output.WriteString(positionedLine(layout.Top, layout.Left, state.Prompt, layout.Width, "\x1b[38;5;213m"))
	}
	for index, line := range layout.Lines {
		row := layout.Top + 1 + index
		if row < 2 || row >= a.screenRows() {
			continue
		}
		// Inline answers occupy rows that do not exist in the remote VT. Clear only
		// those inserted rows before drawing them instead of blanking the whole SSH pane.
		output.WriteString(positionedLine(row, 1, "", a.mainCols, "\x1b[0m"))
		output.WriteString(positionedLine(row, layout.Left, line, layout.Width, a.inlineLineStyle(state, line)))
	}
	return output.String()
}

func (a *App) inlineLineStyle(state *inlineAIState, line string) string {
	if state.Error != "" {
		return "\x1b[38;5;203m"
	}
	if strings.Contains(line, a.approvalButtonsText()) || strings.HasPrefix(line, "> ") {
		return "\x1b[1;38;5;118m"
	}
	if state.Proposal != nil && isReviewRisk(state.Proposal.Risk) {
		riskLabel := "[" + a.riskDisplay(state.Proposal.Risk) + "]"
		if strings.Contains(line, riskLabel) || strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "  ↳ ") || strings.HasPrefix(line, "  …") {
			return "\x1b[1;38;5;203m"
		}
	}
	if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "  ↳ ") || strings.HasPrefix(line, a.t("命令：%s", "")) {
		return "\x1b[1;38;5;220m"
	}
	if strings.HasPrefix(line, "  …") {
		return "\x1b[38;5;214m"
	}
	return "\x1b[38;5;81m"
}

func normalizedAnchorX(value, width int) int {
	if value < 0 || value >= width {
		return 0
	}
	return value
}

func normalizedAnchorY(value, height int) int {
	if height <= 0 {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value >= height {
		return height - 1
	}
	return value
}

func (a *App) screenRows() int {
	if a.rows > 0 {
		return a.rows
	}
	return a.contentRows + 2
}
