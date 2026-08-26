package app

import (
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type screenPoint struct {
	X int
	Y int
}

type screenTextSelection struct {
	Start    screenPoint
	End      screenPoint
	Dragging bool
	Active   bool
}

type screenTextCanvas struct {
	Rows      int
	Cols      int
	CursorRow int
	CursorCol int
	Cells     [][]rune
}

func newScreenTextCanvas(rows, cols int) *screenTextCanvas {
	canvas := &screenTextCanvas{}
	canvas.resize(rows, cols)
	return canvas
}

func (c *screenTextCanvas) resize(rows, cols int) {
	if rows <= 0 || cols <= 0 || (c.Rows == rows && c.Cols == cols) {
		return
	}
	previous := c.Cells
	previousRows, previousCols := c.Rows, c.Cols
	c.Rows, c.Cols = rows, cols
	c.Cells = make([][]rune, rows)
	for row := range c.Cells {
		c.Cells[row] = make([]rune, cols)
		for col := range c.Cells[row] {
			c.Cells[row][col] = ' '
		}
	}
	copyCols := minInt(cols, previousCols)
	for row := 0; row < rows && row < previousRows; row++ {
		copy(c.Cells[row][:copyCols], previous[row][:copyCols])
	}
	if c.CursorRow >= rows {
		c.CursorRow = rows - 1
	}
	if c.CursorCol >= cols {
		c.CursorCol = cols - 1
	}
}

func (c *screenTextCanvas) apply(value string, rows, cols int) {
	c.resize(rows, cols)
	for index := 0; index < len(value); {
		if value[index] == 0x1b {
			consumed := c.consumeEscape(value[index:])
			if consumed < 1 {
				consumed = 1
			}
			index += consumed
			continue
		}
		switch value[index] {
		case '\r':
			c.CursorCol = 0
			index++
			continue
		case '\n':
			c.CursorRow++
			index++
			continue
		case '\t':
			c.CursorCol = (c.CursorCol/8 + 1) * 8
			index++
			continue
		}
		if value[index] < 0x20 || value[index] == 0x7f {
			index++
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		if r == utf8.RuneError && size == 1 {
			index++
			continue
		}
		c.writeRune(r)
		index += size
	}
}

func (c *screenTextCanvas) consumeEscape(value string) int {
	if len(value) < 2 {
		return len(value)
	}
	if value[1] == ']' {
		for index := 2; index < len(value); index++ {
			if value[index] == 0x07 {
				return index + 1
			}
			if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
				return index + 2
			}
		}
		return len(value)
	}
	if value[1] != '[' {
		if len(value) >= 3 && (value[1] == '(' || value[1] == ')') {
			return 3
		}
		return 2
	}
	end := -1
	for index := 2; index < len(value); index++ {
		if value[index] >= 0x40 && value[index] <= 0x7e {
			end = index
			break
		}
	}
	if end < 0 {
		return len(value)
	}
	c.applyCSI(value[2:end], value[end])
	return end + 1
}

func (c *screenTextCanvas) applyCSI(parameters string, final byte) {
	if strings.HasPrefix(parameters, "?") {
		return
	}
	values := []int{0}
	if parameters != "" {
		parts := strings.Split(parameters, ";")
		values = make([]int, len(parts))
		for index, part := range parts {
			values[index], _ = strconv.Atoi(part)
		}
	}
	switch final {
	case 'H', 'f':
		row, col := 1, 1
		if len(values) > 0 && values[0] > 0 {
			row = values[0]
		}
		if len(values) > 1 && values[1] > 0 {
			col = values[1]
		}
		c.CursorRow, c.CursorCol = row-1, col-1
	case 'G':
		col := 1
		if len(values) > 0 && values[0] > 0 {
			col = values[0]
		}
		c.CursorCol = col - 1
	case 'd':
		row := 1
		if len(values) > 0 && values[0] > 0 {
			row = values[0]
		}
		c.CursorRow = row - 1
	case 'K':
		if c.CursorRow < 0 || c.CursorRow >= c.Rows {
			return
		}
		mode := 0
		if len(values) > 0 {
			mode = values[0]
		}
		start, end := c.CursorCol, c.Cols
		if mode == 1 {
			start, end = 0, c.CursorCol+1
		} else if mode == 2 {
			start, end = 0, c.Cols
		}
		start, end = clampInt(start, 0, c.Cols), clampInt(end, 0, c.Cols)
		for col := start; col < end; col++ {
			c.Cells[c.CursorRow][col] = ' '
		}
	}
}

func (c *screenTextCanvas) writeRune(value rune) {
	if c.CursorRow < 0 || c.CursorRow >= c.Rows || c.CursorCol < 0 || c.CursorCol >= c.Cols {
		return
	}
	width := terminalview.CellWidth(value)
	if width < 1 {
		width = 1
	}
	c.Cells[c.CursorRow][c.CursorCol] = value
	for offset := 1; offset < width && c.CursorCol+offset < c.Cols; offset++ {
		c.Cells[c.CursorRow][c.CursorCol+offset] = 0
	}
	c.CursorCol += width
}

func (a *App) beginTextSelection(x, y int) {
	a.leaveNativeSelection()
	a.selectionStatus = ""
	a.textSelection = screenTextSelection{
		Start:    a.clampScreenPoint(x, y),
		End:      a.clampScreenPoint(x, y),
		Dragging: true,
	}
	a.terminalRows = nil
}

func (a *App) updateTextSelection(x, y int) {
	if !a.textSelection.Dragging {
		return
	}
	a.textSelection.End = a.clampScreenPoint(x, y)
	a.textSelection.Active = a.textSelection.End != a.textSelection.Start
	a.terminalRows = nil
	a.dirty = true
}

func (a *App) finishTextSelection(x, y int) bool {
	if !a.textSelection.Dragging {
		return false
	}
	a.updateTextSelection(x, y)
	a.textSelection.Dragging = false
	if !a.textSelection.Active {
		a.textSelection = screenTextSelection{}
	}
	a.dirty = true
	return a.textSelection.Active
}

func (a *App) clearTextSelection() {
	if !a.textSelection.Active && !a.textSelection.Dragging && a.selectionStatus == "" {
		return
	}
	a.textSelection = screenTextSelection{}
	a.selectionStatus = ""
	a.terminalRows = nil
	a.dirty = true
}

func (a *App) copyTextSelection() bool {
	if !a.textSelection.Active || a.selectionCanvas == nil {
		return false
	}
	value := a.selectedScreenText()
	if value == "" {
		return false
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	_, _ = io.WriteString(a.outputWriter(), "\x1b]52;c;"+encoded+"\x07")
	a.selectionStatus = a.t("已复制 %d 个字符", len([]rune(value)))
	a.textSelection = screenTextSelection{}
	a.terminalRows = nil
	a.dirty = true
	return true
}

func (a *App) captureSelectionCanvas(frame string) {
	if a.selectionCanvas == nil {
		a.selectionCanvas = newScreenTextCanvas(a.rows, a.cols)
	}
	a.selectionCanvas.apply(frame, a.rows, a.cols)
}

func (a *App) renderTextSelection(output *strings.Builder) {
	if !a.textSelection.Active || a.selectionCanvas == nil {
		return
	}
	start, end := orderedScreenPoints(a.textSelection.Start, a.textSelection.End)
	for row := start.Y; row <= end.Y; row++ {
		left, right := 1, a.cols
		if row == start.Y {
			left = start.X
		}
		if row == end.Y {
			right = end.X
		}
		if right < left {
			continue
		}
		value := a.selectionCanvas.cellText(row, left, right)
		output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[7m%s\x1b[0m", row, left, value))
	}
}

func (a *App) selectedScreenText() string {
	start, end := orderedScreenPoints(a.textSelection.Start, a.textSelection.End)
	lines := make([]string, 0, end.Y-start.Y+1)
	for row := start.Y; row <= end.Y; row++ {
		left, right := 1, a.cols
		if row == start.Y {
			left = start.X
		}
		if row == end.Y {
			right = end.X
		}
		lines = append(lines, strings.TrimRight(a.selectionCanvas.cellText(row, left, right), " "))
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

func (c *screenTextCanvas) cellText(row, left, right int) string {
	if row < 1 || row > c.Rows {
		return ""
	}
	left, right = clampInt(left, 1, c.Cols), clampInt(right, 1, c.Cols)
	if right < left {
		return ""
	}
	var value strings.Builder
	for col := left - 1; col < right; col++ {
		cell := c.Cells[row-1][col]
		if cell != 0 {
			value.WriteRune(cell)
		}
	}
	return value.String()
}

func (a *App) clampScreenPoint(x, y int) screenPoint {
	return screenPoint{X: clampInt(x, 1, a.cols), Y: clampInt(y, 1, a.rows)}
}

func orderedScreenPoints(left, right screenPoint) (screenPoint, screenPoint) {
	if left.Y > right.Y || (left.Y == right.Y && left.X > right.X) {
		return right, left
	}
	return left, right
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
