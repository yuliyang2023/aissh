package terminalview

import (
	"fmt"
	"strings"

	"github.com/hinshun/vt10x"
)

type Line []vt10x.Glyph

func CaptureLine(view vt10x.View, sourceRow, cols int) Line {
	if view == nil || sourceRow < 0 || cols <= 0 {
		return nil
	}
	view.Lock()
	defer view.Unlock()
	line := make(Line, cols)
	for x := 0; x < cols; x++ {
		line[x] = view.Cell(x, sourceRow)
	}
	return line
}

func CaptureTopLineIfScrolling(view vt10x.View, bottomRow, cols int) (Line, bool) {
	if view == nil || bottomRow < 0 || cols <= 0 {
		return nil, false
	}
	view.Lock()
	defer view.Unlock()
	if view.Cursor().Y < bottomRow || view.Mode()&vt10x.ModeAltScreen != 0 {
		return nil, false
	}
	line := make(Line, cols)
	for x := 0; x < cols; x++ {
		line[x] = view.Cell(x, 0)
	}
	return line, true
}

func Snapshot(view vt10x.View, rows, cols int) []Line {
	if view == nil || rows <= 0 || cols <= 0 {
		return nil
	}
	view.Lock()
	defer view.Unlock()
	lines := make([]Line, rows)
	for y := 0; y < rows; y++ {
		lines[y] = make(Line, cols)
		for x := 0; x < cols; x++ {
			lines[y][x] = view.Cell(x, y)
		}
	}
	return lines
}

func RenderVT(view vt10x.View, top, left, cols, rows int) string {
	if view == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	view.Lock()
	defer view.Unlock()
	var output strings.Builder
	for y := 0; y < rows; y++ {
		output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[0m", top+y, left))
		var currentFG, currentBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
		for x := 0; x < cols; x++ {
			cell := view.Cell(x, y)
			if cell.FG != currentFG || cell.BG != currentBG {
				output.WriteString(colorSequence(cell.FG, cell.BG))
				currentFG, currentBG = cell.FG, cell.BG
			}
			if cell.Char == WidePlaceholder {
				continue
			}
			if cell.Char == 0 {
				output.WriteByte(' ')
			} else {
				output.WriteRune(cell.Char)
			}
		}
		output.WriteString("\x1b[0m")
	}
	return output.String()
}

func RenderVTLine(view vt10x.View, sourceRow, targetRow, left, cols int) string {
	if view == nil || sourceRow < 0 || targetRow < 1 || cols <= 0 {
		return ""
	}
	view.Lock()
	defer view.Unlock()
	var output strings.Builder
	output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[0m", targetRow, left))
	var currentFG, currentBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	for x := 0; x < cols; x++ {
		cell := view.Cell(x, sourceRow)
		if cell.FG != currentFG || cell.BG != currentBG {
			output.WriteString(colorSequence(cell.FG, cell.BG))
			currentFG, currentBG = cell.FG, cell.BG
		}
		if cell.Char == WidePlaceholder {
			continue
		}
		if cell.Char == 0 {
			output.WriteByte(' ')
		} else {
			output.WriteRune(cell.Char)
		}
	}
	output.WriteString("\x1b[0m")
	return output.String()
}

func RenderLine(line Line, targetRow, left, cols int) string {
	if targetRow < 1 || left < 1 || cols <= 0 {
		return ""
	}
	var output strings.Builder
	output.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[0m", targetRow, left))
	var currentFG, currentBG vt10x.Color = vt10x.DefaultFG, vt10x.DefaultBG
	for x := 0; x < cols; x++ {
		cell := vt10x.Glyph{FG: vt10x.DefaultFG, BG: vt10x.DefaultBG}
		if x < len(line) {
			cell = line[x]
		}
		if cell.FG != currentFG || cell.BG != currentBG {
			output.WriteString(colorSequence(cell.FG, cell.BG))
			currentFG, currentBG = cell.FG, cell.BG
		}
		if cell.Char == WidePlaceholder {
			continue
		}
		if cell.Char == 0 {
			output.WriteByte(' ')
		} else {
			output.WriteRune(cell.Char)
		}
	}
	output.WriteString("\x1b[0m")
	return output.String()
}

func colorSequence(fg, bg vt10x.Color) string {
	parts := make([]string, 0, 2)
	if fg == vt10x.DefaultFG {
		parts = append(parts, "39")
	} else if uint32(fg) < 256 {
		parts = append(parts, fmt.Sprintf("38;5;%d", fg))
	}
	if bg == vt10x.DefaultBG {
		parts = append(parts, "49")
	} else if uint32(bg) < 256 {
		parts = append(parts, fmt.Sprintf("48;5;%d", bg))
	}
	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func ClipPad(value string, width int) string {
	if width <= 0 {
		return ""
	}
	var output strings.Builder
	used := 0
	for _, r := range value {
		cellWidth := CellWidth(r)
		if used+cellWidth > width {
			break
		}
		output.WriteRune(r)
		used += cellWidth
	}
	if used < width {
		output.WriteString(strings.Repeat(" ", width-used))
	}
	return output.String()
}

func Wrap(value string, width int) []string {
	if width <= 0 {
		return nil
	}
	value = strings.ReplaceAll(value, "\r", "")
	var lines []string
	for _, source := range strings.Split(value, "\n") {
		if source == "" {
			lines = append(lines, "")
			continue
		}
		var line strings.Builder
		used := 0
		for _, r := range source {
			cellWidth := CellWidth(r)
			if used > 0 && used+cellWidth > width {
				lines = append(lines, line.String())
				line.Reset()
				used = 0
			}
			line.WriteRune(r)
			used += cellWidth
		}
		lines = append(lines, line.String())
	}
	return lines
}
