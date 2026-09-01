package app

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestSidebarToggleExpandsTerminalAndRestoresLayout(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()
	defaultMainCols := application.mainCols

	application.toggleSidebar()
	if !application.sidebarCollapsed || application.mainCols != 139 || application.sidebarVisible() {
		t.Fatalf("collapsed=%v mainCols=%d visible=%v", application.sidebarCollapsed, application.mainCols, application.sidebarVisible())
	}
	if button := application.sidebarToggleButton(); !strings.Contains(button, "展开右栏") {
		t.Fatalf("collapsed button=%q", button)
	}

	application.toggleSidebar()
	if application.sidebarCollapsed || application.mainCols != defaultMainCols || !application.sidebarVisible() {
		t.Fatalf("collapsed=%v mainCols=%d visible=%v", application.sidebarCollapsed, application.mainCols, application.sidebarVisible())
	}
}

func TestCtrlShiftMinusTogglesSidebarOnlyAtShellPrompt(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	application.sshPTY = writer

	shortcut := []byte{0x1f}
	application.handleKeyboardInput(shortcut)
	if application.sidebarCollapsed {
		t.Fatal("Ctrl+Shift+- was intercepted away from a shell prompt")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if string(written) != string(shortcut) {
		t.Fatalf("non-prompt Ctrl+Shift+- was not forwarded to SSH: %q", written)
	}
	application.sshPTY = nil

	application.atPrompt = true
	application.handleKeyboardInput(shortcut)
	if !application.sidebarCollapsed || application.focus != focusTerminal {
		t.Fatalf("collapsed=%v focus=%v", application.sidebarCollapsed, application.focus)
	}

	application.handleKeyboardInput(shortcut)
	if application.sidebarCollapsed {
		t.Fatal("Ctrl+Shift+- did not restore the sidebar")
	}
}

func TestSidebarHeaderButtonCanBeClicked(t *testing.T) {
	application := mouseTestApp(t)
	start := application.sidebarToggleHeaderStart()
	if start <= 0 {
		t.Fatal("sidebar toggle button missing")
	}
	clickMouse(application, start, 1)
	if !application.sidebarCollapsed || application.mainCols != application.cols-1 {
		t.Fatalf("collapsed=%v mainCols=%d", application.sidebarCollapsed, application.mainCols)
	}
	clickMouse(application, application.sidebarToggleHeaderStart(), 1)
	if application.sidebarCollapsed {
		t.Fatal("header button did not expand the sidebar")
	}
}

func TestOpeningSidebarWorkspaceExpandsCollapsedSidebar(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.cols = 140
	application.rows = 36
	application.contentRows = 34
	application.applyPaneLayout()
	application.toggleSidebar()

	application.openAIConversation()
	if application.sidebarCollapsed || application.focus != focusAI || !application.sidebarVisible() {
		t.Fatalf("collapsed=%v focus=%v visible=%v", application.sidebarCollapsed, application.focus, application.sidebarVisible())
	}
}

func TestWideResizeShowsFocusedCompactWorkspace(t *testing.T) {
	application, err := New(Config{SSHArgs: []string{"server"}})
	if err != nil {
		t.Fatal(err)
	}
	application.setViewportSize(140, 30)
	application.toggleSidebar()
	application.setViewportSize(80, 30)
	application.focus = focusAI
	application.setViewportSize(140, 30)

	if application.sidebarCollapsed || !application.sidebarVisible() || application.focus != focusAI {
		t.Fatalf("collapsed=%v visible=%v focus=%v", application.sidebarCollapsed, application.sidebarVisible(), application.focus)
	}
}
