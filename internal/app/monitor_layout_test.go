package app

import (
	"strings"
	"testing"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
)

func TestAutomaticMonitorLayoutUsesGridOnLargeScreens(t *testing.T) {
	application := layoutTestApp(t)
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程", Content: "pid cpu"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘", Content: "/ 20%"})
	application.panels.Open(panel.Panel{ID: "network", Kind: panel.KindMonitor, Title: "网络", Content: "listen 22"})

	if got := application.effectiveMonitorLayout(96, 24); got != monitorLayoutGrid {
		t.Fatalf("layout=%v", got)
	}
	tiles := application.monitorTiles(3, 100, 96, 24)
	if len(tiles) != 4 {
		t.Fatalf("tiles=%d", len(tiles))
	}
	output := application.renderMonitorWorkspace(2, 100, 96, 25)
	for _, want := range []string{"系统", "进程", "磁盘", "网络", "布局:自动", "┌─", "[换]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("grid output missing %q: %q", want, output)
		}
	}
}

func TestMonitorGridClearsUnusedTilesFromPreviousFrame(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayoutGrid
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘"})
	application.panels.Open(panel.Panel{ID: "network", Kind: panel.KindMonitor, Title: "网络"})
	application.panels.Open(panel.Panel{ID: "service", Kind: panel.KindTable, Title: "服务"})

	output := application.renderMonitorWorkspace(2, 100, 96, 25)
	if clear := positionedLine(16, 100, "", 96, "\x1b[0m"); !strings.Contains(output, clear) {
		t.Fatalf("unused grid row was not cleared: %q", output)
	}
}

func TestMonitorGridSlotCanBeAssignedIndependently(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayoutGrid
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘"})
	activeBefore := application.panels.Active().ID

	application.openMonitorSlotPicker(0)
	if application.focus != focusMonitorSlot || application.monitorSlotMode != monitorLayoutGrid {
		t.Fatalf("focus=%v mode=%v", application.focus, application.monitorSlotMode)
	}
	if !application.assignMonitorSlot(1) {
		t.Fatal("slot assignment failed")
	}
	top, left, width, height := application.monitorRegion()
	tiles := application.monitorTiles(top+1, left, width, height-1)
	if len(tiles) == 0 || tiles[0].Panel.ID != "process" {
		t.Fatalf("first tile=%#v", tiles)
	}
	if application.panels.Active().ID != activeBefore {
		t.Fatalf("slot assignment unexpectedly changed active tab to %s", application.panels.Active().ID)
	}
}

func TestMouseOpensMonitorSlotPickerAndSelectsPanel(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayoutGrid
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘"})
	application.focus = focusMonitor
	top, left, width, height := application.monitorRegion()
	tiles := application.monitorTiles(top+1, left, width, height-1)
	if len(tiles) == 0 {
		t.Fatal("monitor tiles missing")
	}
	first := tiles[0]
	clickMouse(application, first.Left+first.Width-3, first.Top)
	if application.focus != focusMonitorSlot || application.monitorSlot != 0 {
		t.Fatalf("focus=%v slot=%d", application.focus, application.monitorSlot)
	}
	// Picker rows: title, blank, system, process.
	clickMouse(application, left+2, top+4)
	if application.focus != focusMonitor {
		t.Fatalf("focus=%v", application.focus)
	}
	tiles = application.monitorTiles(top+1, left, width, height-1)
	if tiles[0].Panel.ID != "process" {
		t.Fatalf("first tile=%s", tiles[0].Panel.ID)
	}
}

func TestMonitorLayoutCyclesWithKeyboardAndMouse(t *testing.T) {
	application := layoutTestApp(t)
	application.focus = focusMonitor
	application.handleMonitorInput([]byte{0x0c})
	if application.monitorLayout != monitorLayoutSingle {
		t.Fatalf("keyboard layout=%v", application.monitorLayout)
	}
	top, _, _, _ := application.monitorRegion()
	clickMouse(application, application.monitorLayoutHeaderStart(), top)
	if application.monitorLayout != monitorLayoutSplit {
		t.Fatalf("mouse layout=%v", application.monitorLayout)
	}

	wantModes := []monitorLayoutMode{
		monitorLayout1x3,
		monitorLayout2x2,
		monitorLayoutGrid,
		monitorLayoutAuto,
	}
	for _, want := range wantModes {
		clickMouse(application, application.monitorLayoutHeaderStart(), top)
		if application.monitorLayout != want {
			t.Fatalf("mouse layout=%v, want %v", application.monitorLayout, want)
		}
	}
}

func TestFixedOneByThreeMonitorLayout(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayout1x3
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘"})

	if got := application.effectiveMonitorLayout(96, 18); got != monitorLayout1x3 {
		t.Fatalf("layout=%v", got)
	}
	tiles := application.monitorTiles(3, 100, 96, 18)
	if len(tiles) != 3 {
		t.Fatalf("tiles=%d", len(tiles))
	}
	for index, tile := range tiles {
		if tile.Top != 3 || tile.Height != 18 || tile.Width != 32 {
			t.Fatalf("tile %d geometry=%+v", index, tile)
		}
		if tile.Left != 100+index*32 {
			t.Fatalf("tile %d left=%d", index, tile.Left)
		}
	}
	if label := application.monitorLayoutLabel(); label != "1×3" {
		t.Fatalf("label=%q", label)
	}
	if got := application.effectiveMonitorLayout(59, 18); got != monitorLayoutSingle {
		t.Fatalf("narrow layout=%v", got)
	}
}

func TestFixedTwoByTwoMonitorLayout(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayout2x2
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程"})
	application.panels.Open(panel.Panel{ID: "disk", Kind: panel.KindMonitor, Title: "磁盘"})
	application.panels.Open(panel.Panel{ID: "network", Kind: panel.KindMonitor, Title: "网络"})

	if got := application.effectiveMonitorLayout(96, 24); got != monitorLayout2x2 {
		t.Fatalf("layout=%v", got)
	}
	tiles := application.monitorTiles(3, 100, 96, 24)
	if len(tiles) != 4 {
		t.Fatalf("tiles=%d", len(tiles))
	}
	wantPositions := [][2]int{{3, 100}, {3, 148}, {15, 100}, {15, 148}}
	for index, tile := range tiles {
		if tile.Top != wantPositions[index][0] || tile.Left != wantPositions[index][1] || tile.Width != 48 || tile.Height != 12 {
			t.Fatalf("tile %d geometry=%+v", index, tile)
		}
	}
	if label := application.monitorLayoutLabel(); label != "2×2" {
		t.Fatalf("label=%q", label)
	}
	if got := application.effectiveMonitorLayout(96, 11); got != monitorLayoutSingle {
		t.Fatalf("short layout=%v", got)
	}
}

func TestMouseSelectsPanelInsideMonitorGrid(t *testing.T) {
	application := layoutTestApp(t)
	application.monitorLayout = monitorLayoutGrid
	application.panels.Open(panel.Panel{ID: "process", Kind: panel.KindTable, Title: "进程", Content: "pid cpu"})
	top, left, width, height := application.monitorRegion()
	tiles := application.monitorTiles(top+1, left, width, height-1)
	var target monitorTileLayout
	for _, tile := range tiles {
		if tile.Panel.ID == "process" {
			target = tile
			break
		}
	}
	if target.Panel == nil {
		t.Fatal("process tile not found")
	}
	clickMouse(application, target.Left+1, target.Top+1)
	if application.panels.Active().ID != "process" || application.focus != focusMonitor {
		t.Fatalf("active=%s focus=%v", application.panels.Active().ID, application.focus)
	}
}

func TestHiddenCompactMonitorUpdateDoesNotRequestFrame(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.compact = true
	application.focus = focusTerminal
	application.dirty = false

	application.handleEvent(monitorResultEvent{panelID: "missing", content: "updated"})
	if application.dirty {
		t.Fatal("hidden compact monitor update requested an unnecessary terminal frame")
	}

	application.focus = focusMonitor
	application.handleEvent(monitorResultEvent{panelID: "missing", content: "updated"})
	if !application.dirty {
		t.Fatal("visible compact monitor update did not request a frame")
	}
}

func layoutTestApp(t *testing.T) *App {
	t.Helper()
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.ensureSidebarVisible()
	application.cols = 260
	application.rows = 42
	application.mainCols = 160
	application.contentRows = 40
	return application
}
