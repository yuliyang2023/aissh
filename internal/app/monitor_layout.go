package app

import (
	"fmt"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
)

type monitorTileLayout struct {
	Panel  *panel.Panel
	Top    int
	Left   int
	Width  int
	Height int
}

func (a *App) cycleMonitorLayout() {
	a.monitorLayout = (a.monitorLayout + 1) % 6
}

func (a *App) monitorLayoutLabel() string {
	switch a.monitorLayout {
	case monitorLayoutSingle:
		return a.t("单屏")
	case monitorLayoutSplit:
		return a.t("双栏")
	case monitorLayout1x3:
		return a.t("1×3")
	case monitorLayout2x2:
		return a.t("2×2")
	case monitorLayoutGrid:
		return a.t("网格")
	default:
		return a.t("自动")
	}
}

func (a *App) monitorLayoutButton() string {
	return a.t(" [布局:%s]", a.monitorLayoutLabel())
}

func (a *App) effectiveMonitorLayout(width, height int) monitorLayoutMode {
	count := len(a.panels.Panels())
	if count <= 1 {
		return monitorLayoutSingle
	}
	switch a.monitorLayout {
	case monitorLayoutSingle:
		return monitorLayoutSingle
	case monitorLayoutSplit:
		if width >= 48 && height >= 7 {
			return monitorLayoutSplit
		}
		return monitorLayoutSingle
	case monitorLayout1x3:
		if width >= 60 && height >= 7 {
			return monitorLayout1x3
		}
		return monitorLayoutSingle
	case monitorLayout2x2:
		if width >= 48 && height >= 12 {
			return monitorLayout2x2
		}
		return monitorLayoutSingle
	case monitorLayoutGrid:
		if width >= 48 && height >= 7 {
			return monitorLayoutGrid
		}
		return monitorLayoutSingle
	default:
		if width >= 68 && height >= 14 {
			return monitorLayoutGrid
		}
		if width >= 54 && height >= 9 {
			return monitorLayoutSplit
		}
		return monitorLayoutSingle
	}
}

func (a *App) monitorHeaderLine(width int) string {
	prefix := a.t("监控 ")
	button := a.monitorLayoutButton()
	tabWidth := width - textWidth(prefix) - textWidth(button)
	if tabWidth < 0 {
		tabWidth = 0
	}
	return terminalview.ClipPad(prefix+a.tabBar(tabWidth)+button, width)
}

func (a *App) monitorLayoutHeaderStart() int {
	_, left, width, _ := a.monitorRegion()
	return left + width - textWidth(a.monitorLayoutButton())
}

func (a *App) monitorTabAreaWidth() int {
	_, _, width, _ := a.monitorRegion()
	value := width - textWidth(a.t("监控 ")) - textWidth(a.monitorLayoutButton())
	if value < 0 {
		return 0
	}
	return value
}

func (a *App) monitorTiles(top, left, width, height int) []monitorTileLayout {
	mode := a.effectiveMonitorLayout(width, height)
	if mode == monitorLayoutSingle || width <= 0 || height <= 0 {
		return nil
	}
	panels := a.panels.Panels()
	if len(panels) == 0 {
		return nil
	}
	columns, rows := 2, 1
	switch mode {
	case monitorLayout1x3:
		columns, rows = 3, 1
	case monitorLayout2x2:
		columns, rows = 2, 2
	case monitorLayoutGrid:
		columns = width / 32
		if columns < 1 {
			columns = 1
		}
		if columns > 3 {
			columns = 3
		}
		if columns > len(panels) {
			columns = len(panels)
		}
		rows = (len(panels) + columns - 1) / columns
		maximumRows := height / 7
		if maximumRows < 1 {
			maximumRows = 1
		}
		if maximumRows > 3 {
			maximumRows = 3
		}
		if rows > maximumRows {
			rows = maximumRows
		}
	default:
		if len(panels) < columns {
			columns = len(panels)
		}
	}
	capacity := columns * rows
	visible := a.visibleMonitorPanels(mode, capacity)
	if len(visible) == 0 {
		return nil
	}
	if mode == monitorLayoutGrid {
		rows = (len(visible) + columns - 1) / columns
	}
	tiles := make([]monitorTileLayout, 0, len(visible))
	for index, item := range visible {
		column := index % columns
		row := index / columns
		cellLeft, cellWidth := dividedSpan(left, width, columns, column)
		cellTop, cellHeight := dividedSpan(top, height, rows, row)
		tiles = append(tiles, monitorTileLayout{Panel: item, Top: cellTop, Left: cellLeft, Width: cellWidth, Height: cellHeight})
	}
	return tiles
}

func (a *App) visibleMonitorPanels(mode monitorLayoutMode, capacity int) []*panel.Panel {
	panels := a.panels.Panels()
	if capacity <= 0 || len(panels) == 0 {
		return nil
	}
	if capacity > len(panels) {
		capacity = len(panels)
	}
	if mode == monitorLayoutSplit {
		visible := make([]*panel.Panel, 0, capacity)
		start := a.panels.ActiveIndex()
		for index := 0; index < capacity; index++ {
			visible = append(visible, panels[(start+index)%len(panels)])
		}
		return a.applyMonitorSlotAssignments(mode, visible)
	}
	pageStart := (a.panels.ActiveIndex() / capacity) * capacity
	pageEnd := pageStart + capacity
	if pageEnd > len(panels) {
		pageEnd = len(panels)
	}
	visible := append([]*panel.Panel(nil), panels[pageStart:pageEnd]...)
	return a.applyMonitorSlotAssignments(mode, visible)
}

func (a *App) applyMonitorSlotAssignments(mode monitorLayoutMode, visible []*panel.Panel) []*panel.Panel {
	assignments := a.monitorSlots[mode]
	if len(assignments) == 0 {
		return visible
	}
	for slot, panelID := range assignments {
		if slot < 0 || slot >= len(visible) {
			continue
		}
		if item := a.panels.ByID(panelID); item != nil {
			visible[slot] = item
		}
	}
	return visible
}

func dividedSpan(start, size, count, index int) (int, int) {
	base := size / count
	remainder := size % count
	offset := index * base
	if index < remainder {
		offset += index
		base++
	} else {
		offset += remainder
	}
	return start + offset, base
}

func (a *App) renderMonitorTiles(tiles []monitorTileLayout) string {
	var output strings.Builder
	active := a.panels.Active()
	for _, tile := range tiles {
		if tile.Width < 8 || tile.Height < 3 {
			continue
		}
		isActive := active != nil && active.ID == tile.Panel.ID
		title := tile.Panel.Title
		if tile.Panel.Unread {
			title += "*"
		}
		if isActive {
			title = "● " + title
		}
		innerWidth := tile.Width - 4
		topLine := "┌─ " + terminalview.ClipPad(title, tile.Width-5) + "─┐"
		changeButton := a.t("[换]")
		buttonWidth := textWidth(changeButton)
		if tile.Width >= buttonWidth+8 {
			topLine = "┌─ " + terminalview.ClipPad(title, tile.Width-buttonWidth-5) + " " + changeButton + "┐"
		}
		style := "\x1b[38;5;244m"
		if isActive {
			style = "\x1b[1;38;5;81m"
		}
		output.WriteString(positionedLine(tile.Top, tile.Left, topLine, tile.Width, style))
		lines := a.panelLines(tile.Panel, innerWidth)
		viewport := tile.Height - 2
		offset := a.clampedPanelScroll(tile.Panel.ID, len(lines), viewport)
		visible := fitTailOffset(lines, viewport, offset)
		for row := 0; row < viewport; row++ {
			value := ""
			if row < len(visible) {
				value = visible[row]
			}
			framed := "│ " + terminalview.ClipPad(value, innerWidth) + " │"
			output.WriteString(positionedLine(tile.Top+1+row, tile.Left, framed, tile.Width, "\x1b[0m"))
		}
		output.WriteString(renderScrollbar(tile.Top+1, tile.Left+tile.Width-2, viewport, len(lines), viewport, offset))
		output.WriteString(positionedLine(tile.Top+tile.Height-1, tile.Left, "└"+strings.Repeat("─", tile.Width-2)+"┘", tile.Width, style))
	}
	return output.String()
}

func (a *App) monitorSlotButtonTarget(x, y int) (int, bool) {
	top, left, width, height := a.monitorRegion()
	for slot, tile := range a.monitorTiles(top+1, left, width, height-1) {
		buttonWidth := textWidth(a.t("[换]"))
		if tile.Width < buttonWidth+8 || y != tile.Top {
			continue
		}
		if x >= tile.Left+tile.Width-buttonWidth-1 && x < tile.Left+tile.Width-1 {
			return slot, true
		}
	}
	return 0, false
}

func (a *App) openMonitorSlotPicker(slot int) {
	_, _, width, height := a.monitorRegion()
	mode := a.effectiveMonitorLayout(width, height-1)
	if mode == monitorLayoutSingle {
		return
	}
	a.monitorSlot = slot
	a.monitorSlotMode = mode
	a.focus = focusMonitorSlot
}

func (a *App) assignMonitorSlot(panelIndex int) bool {
	panels := a.panels.Panels()
	if a.monitorSlot < 0 || panelIndex < 0 || panelIndex >= len(panels) {
		return false
	}
	if a.monitorSlots[a.monitorSlotMode] == nil {
		a.monitorSlots[a.monitorSlotMode] = make(map[int]string)
	}
	item := panels[panelIndex]
	a.monitorSlots[a.monitorSlotMode][a.monitorSlot] = item.ID
	a.monitorSlot = -1
	a.focus = focusMonitor
	return true
}

func (a *App) monitorSlotPickerLines(width int) []string {
	lines := []string{
		a.t("选择第 %d 块显示内容 · %s布局", a.monitorSlot+1, a.monitorLayoutModeLabel(a.monitorSlotMode)),
		"",
	}
	assigned := ""
	if slots := a.monitorSlots[a.monitorSlotMode]; slots != nil {
		assigned = slots[a.monitorSlot]
	}
	if assigned == "" {
		top, left, regionWidth, height := a.monitorRegion()
		tiles := a.monitorTiles(top+1, left, regionWidth, height-1)
		if a.monitorSlot >= 0 && a.monitorSlot < len(tiles) {
			assigned = tiles[a.monitorSlot].Panel.ID
		}
	}
	for index, item := range a.panels.Panels() {
		prefix := "  "
		if index < 9 {
			prefix = fmt.Sprintf("%d ", index+1)
		}
		marker := " "
		if item.ID == assigned {
			marker = "●"
		}
		lines = append(lines, terminalview.ClipPad(prefix+marker+" "+item.Title, width))
	}
	lines = append(lines, "", a.t("Esc 取消"))
	return lines
}

func (a *App) monitorLayoutModeLabel(mode monitorLayoutMode) string {
	switch mode {
	case monitorLayoutSplit:
		return a.t("双栏")
	case monitorLayout1x3:
		return a.t("1×3")
	case monitorLayout2x2:
		return a.t("2×2")
	case monitorLayoutGrid:
		return a.t("网格")
	case monitorLayoutSingle:
		return a.t("单屏")
	default:
		return a.t("自动")
	}
}

func (a *App) panelLines(item *panel.Panel, width int) []string {
	if item == nil {
		return []string{a.t("没有监控面板")}
	}
	switch item.Kind {
	case panel.KindReport:
		content := firstText(item.Content, a.t("调查报告将在这里生成。"))
		return terminalview.Wrap(content, width)
	default:
		return a.monitorLines(item, width)
	}
}

func (a *App) monitorTileAt(x, y int) *panel.Panel {
	top, left, width, height := a.monitorRegion()
	if y <= top {
		return nil
	}
	for _, tile := range a.monitorTiles(top+1, left, width, height-1) {
		if pointInRegion(x, y, tile.Top, tile.Left, tile.Width, tile.Height) {
			return tile.Panel
		}
	}
	return nil
}
