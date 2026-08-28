package app

import (
	"bytes"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

var (
	terminalPasteStart = []byte("\x1b[200~")
	terminalPasteEnd   = []byte("\x1b[201~")
)

const (
	bracketedPasteEnable  = "\x1b[?2004h"
	bracketedPasteDisable = "\x1b[?2004l"
)

func (a *App) handleInput(data []byte) {
	if len(data) == 0 {
		return
	}
	if a.zmodem != nil {
		a.handleZmodemInput(data)
		return
	}
	if a.nativeSelection {
		// A right-button release may already be queued when mouse reporting is
		// disabled. Do not let that stale event immediately leave selection mode.
		if bytes.HasPrefix(data, sgrMousePrefix) {
			return
		}
		a.leaveNativeSelection()
	}
	if !a.sensitive && bytes.Contains(data, []byte{0x0f}) { // Ctrl+O
		filtered := make([]byte, 0, len(data))
		for _, value := range data {
			if value == 0x0f && a.expandCommandShortcut() {
				continue
			}
			filtered = append(filtered, value)
		}
		data = filtered
		if len(data) == 0 {
			return
		}
	}
	redraw := a.focus != focusTerminal || a.aiLineCapture || a.inlineAI != nil || a.proposal != nil || a.terminalScroll > 0 || terminalInputHasLocalAction(data)
	a.handleMixedInput(data)
	if redraw || a.focus != focusTerminal || a.aiLineCapture || a.inlineAI != nil || a.proposal != nil || a.terminalScroll > 0 {
		a.dirty = true
	}
}

func terminalInputHasLocalAction(data []byte) bool {
	for _, value := range data {
		switch value {
		case 0x0c, 0x0e, 0x14: // Ctrl+L, Ctrl+N, Ctrl+T
			return true
		}
	}
	return false
}

func (a *App) handleKeyboardInput(data []byte) {
	if a.textSelection.Active || a.textSelection.Dragging || a.selectionStatus != "" {
		a.clearTextSelection()
	}
	switch a.focus {
	case focusApproval:
		a.handleApprovalInput(data)
	case focusAI:
		a.handleAIInput(data)
	case focusAIHistory:
		a.handleAIHistoryInput(data)
	case focusMonitor:
		a.handleMonitorInput(data)
	case focusMonitorSlot:
		a.handleMonitorSlotInput(data)
	case focusPanelMenu:
		a.handlePanelMenuInput(data)
	case focusCustomPanel:
		a.handleCustomPanelInput(data)
	default:
		a.handleTerminalInput(data)
	}
}

func (a *App) handleTerminalInput(data []byte) {
	if a.terminalScroll > 0 {
		a.terminalScroll = 0
		a.terminalRows = nil
	}
	if a.sensitive {
		a.handleSensitiveTerminalInput(data)
		return
	}
	if a.inlineAI != nil {
		if a.agentBusy {
			if len(data) > 0 {
				switch data[0] {
				case 0x0c:
					a.clearInlineTerminalOutput()
					if a.sshPTY != nil {
						_, _ = a.sshPTY.Write(data[:1])
					}
				}
			}
			return
		}
		if a.proposal == nil {
			a.archiveInlineAI()
		}
	}
	for len(data) > 0 {
		if a.terminalPasteMode {
			a.terminalPasteBuffer = append(a.terminalPasteBuffer, data...)
			end := bytes.Index(a.terminalPasteBuffer, terminalPasteEnd)
			if end < 0 {
				return
			}
			payload := append([]byte(nil), a.terminalPasteBuffer[:end]...)
			data = append([]byte(nil), a.terminalPasteBuffer[end+len(terminalPasteEnd):]...)
			a.terminalPasteBuffer = nil
			a.terminalPasteMode = false
			if a.finishTerminalPaste(payload) {
				return
			}
			continue
		}

		start := bytes.Index(data, terminalPasteStart)
		if start < 0 {
			a.handleTerminalBytes(data)
			return
		}
		if start > 0 {
			a.handleTerminalBytes(data[:start])
			if a.inlineAI != nil && a.agentBusy {
				return
			}
		}
		a.captureLineAnchor()
		a.terminalPasteMode = true
		a.terminalPasteBuffer = nil
		data = data[start+len(terminalPasteStart):]
	}
}

// handleSensitiveTerminalInput keeps password and passphrase entry out of the
// command/AI input state. The outer terminal has bracketed paste enabled, but
// OpenSSH's password reader does not expect those wrapper bytes, so only the
// pasted payload may be forwarded to the SSH PTY.
func (a *App) handleSensitiveTerminalInput(data []byte) {
	a.lineInput = nil
	a.lineAnchored = false
	a.lineReliable = true
	a.aiLineCapture = false

	for len(data) > 0 {
		if a.terminalPasteMode {
			a.terminalPasteBuffer = append(a.terminalPasteBuffer, data...)
			end := bytes.Index(a.terminalPasteBuffer, terminalPasteEnd)
			if end < 0 {
				return
			}
			payload := append([]byte(nil), a.terminalPasteBuffer[:end]...)
			data = append([]byte(nil), a.terminalPasteBuffer[end+len(terminalPasteEnd):]...)
			a.terminalPasteBuffer = nil
			a.terminalPasteMode = false
			if a.sshPTY != nil {
				_, _ = a.sshPTY.Write(payload)
			}
			continue
		}

		start := bytes.Index(data, terminalPasteStart)
		if start < 0 {
			if a.sshPTY != nil {
				_, _ = a.sshPTY.Write(data)
			}
			return
		}
		if start > 0 && a.sshPTY != nil {
			_, _ = a.sshPTY.Write(data[:start])
		}
		a.terminalPasteMode = true
		a.terminalPasteBuffer = nil
		data = data[start+len(terminalPasteStart):]
	}
}

func (a *App) handleTerminalBytes(data []byte) {
	for len(data) > 0 {
		size := 1
		if data[0] >= utf8.RuneSelf {
			_, decodedSize := utf8.DecodeRune(data)
			if decodedSize > 1 {
				size = decodedSize
			}
		}
		unit := append([]byte(nil), data[:size]...)
		data = data[size:]
		value := unit[0]
		if size > 1 {
			r, _ := utf8.DecodeRune(unit)
			a.captureLineAnchor()
			if a.atPrompt && a.lineReliable && len(a.lineInput) == 0 && unicode.Is(unicode.Han, r) {
				a.aiLineCapture = true
			}
			if a.lineReliable {
				a.lineInput = append(a.lineInput, unit...)
			}
			if !a.aiLineCapture {
				_, _ = a.sshPTY.Write(unit)
			}
			continue
		}
		switch value {
		case 0x03: // Ctrl+C
			if a.aiLineCapture {
				a.lineInput = nil
				a.lineAnchored = false
				a.lineReliable = true
				a.aiLineCapture = false
				continue
			}
		case 0x14: // Ctrl+T
			if a.atPrompt {
				a.panels.Next()
				if a.compact {
					a.focus = focusMonitor
				}
				continue
			}
		case 0x0e: // Ctrl+N
			if a.atPrompt {
				a.focus = focusPanelMenu
				a.menuStatus = ""
				continue
			}
		case 0x0c: // Ctrl+L
			a.clearInlineTerminalOutput()
		case '\r', '\n':
			a.submitTerminalLine(value)
			if a.inlineAI != nil && a.agentBusy {
				return
			}
			continue
		case 0x08, 0x7f:
			if a.lineReliable {
				a.lineInput = removeLastUTF8Rune(a.lineInput)
			}
		case 0x15: // Ctrl+U
			a.lineInput = nil
			a.lineAnchored = false
			a.lineReliable = true
			if a.aiLineCapture {
				a.aiLineCapture = false
				continue
			}
		case 0x1b:
			captured := a.aiLineCapture
			a.lineInput = nil
			a.lineAnchored = false
			a.lineReliable = false
			a.aiLineCapture = false
			if captured {
				continue
			}
		}
		if value >= 0x20 && value != 0x7f && a.lineReliable {
			a.captureLineAnchor()
			a.lineInput = append(a.lineInput, value)
		}
		if !a.aiLineCapture {
			_, _ = a.sshPTY.Write(unit)
		}
	}
}

func (a *App) finishTerminalPaste(payload []byte) bool {
	value := strings.ReplaceAll(string(payload), "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	captured := a.aiLineCapture
	line := string(a.lineInput) + value
	if a.atPrompt && a.lineReliable && !a.sensitive {
		if prompt, ok := trigger.IsAIInput(line); ok {
			if !captured && len(a.lineInput) > 0 && a.sshPTY != nil {
				_, _ = a.sshPTY.Write([]byte{0x15})
			}
			a.lineInput = nil
			a.aiLineCapture = false
			a.lineReliable = true
			a.startInlineAI(prompt, line, true)
			return true
		}
	}

	a.lineInput = nil
	a.lineAnchored = false
	a.lineReliable = false
	a.aiLineCapture = false
	if a.sshPTY != nil {
		_, _ = a.sshPTY.Write(terminalPasteStart)
		_, _ = a.sshPTY.Write(payload)
		_, _ = a.sshPTY.Write(terminalPasteEnd)
	}
	return false
}

func (a *App) submitTerminalLine(newline byte) {
	line := string(a.lineInput)
	a.lineInput = nil
	captured := a.aiLineCapture
	a.aiLineCapture = false
	reliable := a.lineReliable
	a.lineReliable = true
	if a.atPrompt && reliable && !a.sensitive {
		if _, ok := parseLocalWorkspaceCommand(line); ok {
			if !captured && a.sshPTY != nil {
				_, _ = a.sshPTY.Write([]byte{0x15})
			}
			a.handleLocalWorkspaceCommand(line, true)
			a.pendingError = nil
			return
		}
		if prompt, ok := trigger.IsAIInput(line); ok {
			if !captured {
				_, _ = a.sshPTY.Write([]byte{0x15})
			}
			a.startInlineAI(prompt, line, captured)
			return
		}
		if command, ok := trigger.ForceShellInput(line); ok {
			_, _ = a.sshPTY.Write([]byte{0x15})
			a.beginCommandOutput(command)
			_, _ = a.sshPTY.Write([]byte(command + "\r"))
			a.lastCommand = command
			a.atPrompt = false
			a.lineAnchored = false
			return
		}
		if isTerminalClearCommand(line) {
			a.clearInlineTerminalOutput()
		}
	}
	if captured {
		if newline == '\n' {
			newline = '\r'
		}
		a.beginCommandOutput(line)
		_, _ = a.sshPTY.Write([]byte(line))
		_, _ = a.sshPTY.Write([]byte{newline})
		a.lastCommand = strings.TrimSpace(line)
		a.atPrompt = false
		a.pendingError = nil
		a.lineAnchored = false
		return
	}
	if newline == '\n' {
		newline = '\r'
	}
	a.beginCommandOutput(line)
	_, _ = a.sshPTY.Write([]byte{newline})
	a.lastCommand = strings.TrimSpace(line)
	a.atPrompt = false
	a.pendingError = nil
	a.lineAnchored = false
}

func (a *App) captureLineAnchor() {
	if a.lineAnchored || !a.atPrompt || !a.lineReliable || len(a.lineInput) != 0 || a.virtual == nil {
		return
	}
	cursor := a.virtual.Cursor()
	a.lineAnchorX = cursor.X
	a.lineAnchorY = cursor.Y
	a.lineAnchored = true
}

func (a *App) handleAIInput(data []byte) {
	for len(data) > 0 {
		if data[0] == 0x1b {
			if consumed, handled := a.consumeAIEditorEscape(data); handled {
				data = data[consumed:]
				continue
			}
			a.focus = focusTerminal
			a.aiPasteMode = false
			return
		}
		value := data[0]
		data = data[1:]
		switch value {
		case 0x14: // Ctrl+T
			a.focus = focusMonitor
			return
		case 0x0e: // Ctrl+N
			a.focus = focusPanelMenu
			return
		case 0x12: // Ctrl+R
			a.openAIHistory()
			return
		case '\r':
			if a.aiPasteMode {
				a.insertAIEditorRune('\n')
				if len(data) > 0 && data[0] == '\n' {
					data = data[1:]
				}
				continue
			}
			prompt := strings.TrimSpace(string(a.editor))
			if prompt != "" {
				if a.handleLocalWorkspaceCommand(prompt, false) {
					a.editor = nil
					a.aiEditorCursor = 0
					return
				}
			}
			if prompt != "" && !a.agentBusy {
				a.editor = nil
				a.aiEditorCursor = 0
				a.startAI(prompt, true)
			}
			return
		case '\n': // Ctrl+J; terminals that distinguish Shift+Enter use an escape sequence.
			a.insertAIEditorRune('\n')
			continue
		case 0x08, 0x7f:
			a.backspaceAIEditorRune()
			continue
		case 0x01: // Ctrl+A
			a.moveAIEditorHome()
			continue
		case 0x02: // Ctrl+B
			a.moveAIEditorCursor(-1)
			continue
		case 0x04: // Ctrl+D
			a.deleteAIEditorRune()
			continue
		case 0x05: // Ctrl+E
			a.moveAIEditorEnd()
			continue
		case 0x06: // Ctrl+F
			a.moveAIEditorCursor(1)
			continue
		case 0x15: // Ctrl+U
			a.editor = nil
			a.aiEditorCursor = 0
			continue
		}
		if value < 0x20 {
			continue
		}
		r, size := utf8.DecodeRune(append([]byte{value}, data...))
		if r == utf8.RuneError && size == 1 {
			continue
		}
		a.insertAIEditorRune(r)
		if size > 1 {
			data = data[size-1:]
		}
	}
}

func (a *App) handleAIHistoryInput(data []byte) {
	if bytes.HasPrefix(data, []byte("\x1b[A")) {
		a.moveHistorySelection(-1)
		return
	}
	if bytes.HasPrefix(data, []byte("\x1b[B")) {
		a.moveHistorySelection(1)
		return
	}
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case 0x1b:
		a.focus = focusAI
	case '\r', '\n':
		a.selectHistoryConversation(a.historySelection)
	case 'j', 'J':
		a.moveHistorySelection(1)
	case 'k', 'K':
		a.moveHistorySelection(-1)
	case 'n', 'N':
		a.selectHistoryConversation(0)
	}
}

func (a *App) handleMonitorInput(data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case 0x1b:
		a.focus = focusTerminal
	case 0x14:
		a.panels.Next()
	case 0x0c: // Ctrl+L
		a.cycleMonitorLayout()
	case 0x0e:
		a.focus = focusPanelMenu
	}
}

func (a *App) handleMonitorSlotInput(data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case 0x1b:
		a.monitorSlot = -1
		a.focus = focusMonitor
	case 0x14:
		a.monitorSlot = -1
		a.panels.Next()
		a.focus = focusMonitor
	case 0x0c:
		a.monitorSlot = -1
		a.cycleMonitorLayout()
		a.focus = focusMonitor
	case 0x0e:
		a.monitorSlot = -1
		a.focus = focusPanelMenu
	default:
		if data[0] >= '1' && data[0] <= '9' {
			a.assignMonitorSlot(int(data[0] - '1'))
		}
	}
}

func (a *App) handleApprovalInput(data []byte) {
	if a.proposal == nil {
		a.focus = focusTerminal
		return
	}
	if a.agentBusy {
		return
	}
	for len(data) > 0 {
		value := data[0]
		data = data[1:]
		switch value {
		case '\r', '\n':
			choice, ok := approvalChoice(string(a.approvalInput))
			if !ok {
				a.approvalStatus = a.t("请输入 yes 或 no 后回车")
				return
			}
			a.approvalInput = nil
			a.approvalStatus = ""
			if choice {
				a.approveProposal()
			} else {
				a.rejectProposal()
			}
			return
		case 0x08, 0x7f:
			a.approvalInput = appendRuneBackspace(a.approvalInput)
			a.approvalStatus = ""
			continue
		case 0x15:
			a.approvalInput = nil
			a.approvalStatus = ""
			continue
		case 0x1b:
			a.approvalStatus = a.t("请选择 yes（执行）或 no（拒绝）")
			continue
		}
		if value < 0x20 {
			continue
		}
		r, size := utf8.DecodeRune(append([]byte{value}, data...))
		if r == utf8.RuneError && size == 1 {
			continue
		}
		a.approvalInput = append(a.approvalInput, r)
		a.approvalStatus = ""
		if size > 1 {
			data = data[size-1:]
		}
	}
}

func approvalChoice(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes", "是", "执行", "确认":
		return true, true
	case "n", "no", "否", "拒绝", "取消":
		return false, true
	default:
		return false, false
	}
}

func (a *App) handlePanelMenuInput(data []byte) {
	if len(data) == 0 {
		return
	}
	switch data[0] {
	case 0x1b:
		a.focus = focusTerminal
	case '1':
		a.panels.Activate("system")
		a.focus = focusTerminal
	case '2':
		a.openUserPanel(a.t("进程"), panel.KindTable, panel.BuiltinProcessCommand, 3*time.Second)
	case '3':
		a.openUserPanel(a.t("磁盘"), panel.KindMonitor, panel.BuiltinDiskCommand, 10*time.Second)
	case '4':
		a.openUserPanel(a.t("网络"), panel.KindMonitor, panel.BuiltinNetworkCommand, 3*time.Second)
	case '5':
		a.openUserPanel(a.t("服务"), panel.KindTable, panel.BuiltinServiceCommand, 10*time.Second)
	case '6', '7', 'c', 'C':
		a.focus = focusCustomPanel
		a.editor = nil
		a.menuStatus = a.t("输入只读监控命令")
	}
}

func (a *App) handleCustomPanelInput(data []byte) {
	for len(data) > 0 {
		value := data[0]
		data = data[1:]
		switch value {
		case 0x1b:
			a.focus = focusTerminal
			a.editor = nil
			return
		case '\r', '\n':
			command := strings.TrimSpace(string(a.editor))
			if !policy.IsReadOnlyMonitorCommand(command) {
				a.menuStatus = a.t("命令不是只读命令，请修改后重试")
				return
			}
			a.openUserPanel(a.t("自定义"), panel.KindMonitor, command, 3*time.Second)
			a.editor = nil
			return
		case 0x08, 0x7f:
			a.editor = appendRuneBackspace(a.editor)
			continue
		case 0x15:
			a.editor = nil
			continue
		}
		if unicode.IsControl(rune(value)) {
			continue
		}
		r, size := utf8.DecodeRune(append([]byte{value}, data...))
		if r == utf8.RuneError && size == 1 {
			continue
		}
		a.editor = append(a.editor, r)
		if size > 1 {
			data = data[size-1:]
		}
	}
}

func (a *App) openUserPanel(title string, kind panel.Kind, command string, interval time.Duration) {
	item := a.panels.Open(panel.Panel{
		Kind:        kind,
		Origin:      panel.OriginUser,
		Title:       title,
		Command:     command,
		Interval:    interval,
		AllowAIRead: false,
		Target:      a.parsed.Target,
	})
	a.panels.Activate(item.ID)
	if a.compact {
		a.focus = focusMonitor
	} else {
		a.focus = focusTerminal
	}
	a.monitorLast[item.ID] = time.Time{}
}

func removeLastUTF8Rune(value []byte) []byte {
	if len(value) == 0 {
		return value
	}
	_, size := utf8.DecodeLastRune(value)
	if size <= 0 {
		size = 1
	}
	return value[:len(value)-size]
}

func (a *App) openAIConversation() {
	a.focus = focusAI
	a.aiExpanded = true
	a.clampAIEditorCursor()
}

func (a *App) inputStatus() string {
	if a.textSelection.Active {
		return a.t("已选择文本：普通右键复制；单击或键盘输入取消选择")
	}
	if a.selectionStatus != "" {
		return a.selectionStatus
	}
	if a.nativeSelection {
		return a.t("文本选择模式：拖动选择后右键复制；按任意键恢复 aissh 鼠标操作")
	}
	if a.focus == focusTerminal && a.terminalScroll > 0 {
		return a.t("SSH 历史  ↑ %d / %d 行  滚轮向下回到底部；键盘输入也会返回实时终端", a.terminalScroll, len(a.terminalHistory))
	}
	if a.inlineAI != nil && a.focus == focusTerminal {
		if a.agentBusy {
			frame := inlineSpinnerFrames[a.inlineAI.Spinner%len(inlineSpinnerFrames)]
			return a.t("AI %s 本机 %s 正在处理；点击左侧 AI 内容可查看完整对话", frame, a.agent.Name())
		}
		if a.proposal == nil {
			return a.t("AI 回答已显示在原输入下方；继续输入即可返回 SSH，点击内容查看完整对话")
		}
	}
	switch a.focus {
	case focusApproval:
		return a.t("AI 命令确认：输入 yes 执行，输入 no 拒绝，然后按 Enter")
	case focusAI:
		return a.t("AI 对话  Enter发送  Shift+Enter/Ctrl+J换行  方向键编辑  Ctrl+R历史会话  Esc返回SSH")
	case focusAIHistory:
		return a.t("AI 历史会话  ↑/↓选择  Enter继续  N新建  Esc返回对话")
	case focusMonitor:
		return a.t("监控面板  Ctrl+T切换Tab  Ctrl+L切换布局  点击右下AI对话  Ctrl+N新增  Esc返回SSH")
	case focusMonitorSlot:
		return a.t("选择该分屏显示的监控：按数字或鼠标点击；Esc 取消")
	case focusPanelMenu:
		return a.t("新增: 1系统 2进程 3磁盘 4网络 5服务 6自定义  Esc取消")
	case focusCustomPanel:
		return a.t("%s  Enter创建  Esc取消", a.menuStatus)
	default:
		return a.t("鼠标可点击AI对话  Ctrl+T监控Tab  Ctrl+N新增监控  中文或 /ai 自动提问")
	}
}
