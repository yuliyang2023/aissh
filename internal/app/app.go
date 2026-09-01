package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hinshun/vt10x"
	"golang.org/x/term"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/history"
	app_i18n "git.example.com/ai/wrd-ai-cli/internal/i18n"
	"git.example.com/ai/wrd-ai-cli/internal/monitor"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
	"git.example.com/ai/wrd-ai-cli/internal/sshwrap"
	"git.example.com/ai/wrd-ai-cli/internal/terminalview"
	"git.example.com/ai/wrd-ai-cli/internal/trigger"
)

type Config struct {
	Version             string
	SSHPath             string
	SSHArgs             []string
	AgentProvider       string
	AgentURL            string
	AgentToken          string
	CodexPath           string
	CodexModel          string
	ClaudePath          string
	ClaudeModel         string
	Language            string
	HistoryDir          string
	Workspace           string
	WorkspaceWritable   bool
	ModelURL            string
	ModelAPIKey         string
	ModelName           string
	ScrollbackLines     int
	AIContextChars      int
	AITimeout           time.Duration
	SSHKeepalive        time.Duration
	DisableSSHKeepalive bool
	ShellTMOUT          time.Duration
}

const terminalFrameInterval = time.Second / 30
const DefaultScrollbackLines = 20000
const DefaultAIContextChars = agent.DefaultOutputContextChars
const DefaultAITimeout = 5 * time.Minute
const DefaultSSHKeepalive = 30 * time.Second

type App struct {
	config Config
	parsed sshwrap.Parsed

	ctx    context.Context
	cancel context.CancelFunc
	events chan interface{}

	sshCommand   *exec.Cmd
	sshPTY       io.ReadWriteCloser
	controlDir   string
	controlPath  string
	monitor      monitor.Runner
	virtual      vt10x.Terminal
	transformer  terminalview.WideTransformer
	terminalDone bool
	screenOut    io.Writer
	zmodem       *zmodemPassthrough
	zmodemTail   []byte

	cols             int
	rows             int
	mainCols         int
	contentRows      int
	compact          bool
	sidebarCollapsed bool

	panels                  *panel.Manager
	monitorViaPrimary       bool
	inBandMonitor           *inBandMonitorState
	monitorDeferredInput    []byte
	monitorSequence         uint64
	monitorLayout           monitorLayoutMode
	monitorLast             map[string]time.Time
	monitorActive           map[string]bool
	panelScroll             map[string]int
	monitorSlots            map[monitorLayoutMode]map[int]string
	monitorSlot             int
	monitorSlotMode         monitorLayoutMode
	focus                   focusMode
	menuStatus              string
	editor                  []rune
	aiEditorCursor          int
	aiPasteMode             bool
	approvalInput           []rune
	approvalStatus          string
	approvalCommandExpanded bool
	aiLineCapture           bool
	lineInput               []byte
	lineCursor              int
	lineAnchorX             int
	lineAnchorY             int
	lineAnchored            bool
	lineReliable            bool
	terminalKeyCarry        []byte
	terminalPasteMode       bool
	terminalPasteBuffer     []byte
	atPrompt                bool
	sensitive               bool
	lastCommand             string
	outputTail              *rollingBuffer
	recentOutput            *contextBuffer
	errorCarry              string
	pendingError            *trigger.ErrorMatch

	agent                 agent.Provider
	agentBusy             bool
	agentInline           *inlineAIState
	agentSessionID        string
	agentStatus           string
	agentThought          string
	agentLanguage         string
	agentSeedContext      string
	promptIdleSince       time.Time
	chat                  []chatMessage
	proposal              *agent.Proposal
	aiExpanded            bool
	aiScroll              int
	inlineAI              *inlineAIState
	inlineHistory         []*inlineAIState
	execution             *executionState
	localizer             app_i18n.Localizer
	historyStore          *history.Store
	historyTarget         string
	historyWarned         bool
	historyConversations  []history.Conversation
	historyConversationID string
	historySelection      int
	mouseCarry            []byte
	mouseMode             bool
	mouseReporting        bool
	mouseScrollbarDrag    scrollbarDragState
	clearCarry            []byte
	terminalRows          map[int]string
	terminalHistory       []terminalview.Line
	terminalScroll        int

	dirty bool
}

func New(config Config) (*App, error) {
	if strings.TrimSpace(config.SSHPath) == "" {
		config.SSHPath = "ssh"
	}
	if config.ScrollbackLines <= 0 {
		config.ScrollbackLines = DefaultScrollbackLines
	}
	if config.AIContextChars <= 0 {
		config.AIContextChars = DefaultAIContextChars
	}
	if config.AITimeout <= 0 {
		config.AITimeout = DefaultAITimeout
	}
	if config.DisableSSHKeepalive {
		config.SSHKeepalive = 0
	} else if config.SSHKeepalive <= 0 {
		config.SSHKeepalive = DefaultSSHKeepalive
	}
	parsed, err := sshwrap.Parse(config.SSHArgs)
	if err != nil {
		return nil, err
	}
	if len(parsed.RemoteCommand) > 0 {
		return nil, fmt.Errorf("aissh interactive workspace does not accept a remote command yet")
	}
	localizer := app_i18n.New(config.Language)
	config.Language = localizer.Language()
	mode := agentMode(config)
	if mode == "deepseek" {
		config.ModelURL = firstText(config.ModelURL, "https://api.deepseek.com")
		config.ModelName = firstText(config.ModelName, "deepseek-v4-pro")
	}
	if mode != "codex" && mode != "local" && mode != "claude" {
		config.Workspace = ""
		config.WorkspaceWritable = false
	}
	workspace, err := resolveLocalWorkspace(config.Workspace)
	if err != nil {
		return nil, fmt.Errorf("%s", localizer.T("无法访问本机代码工作区：%s", err.Error()))
	}
	config.Workspace = workspace
	ctx, cancel := context.WithCancel(context.Background())
	provider, err := newAgentProvider(config)
	if err != nil {
		cancel()
		return nil, err
	}
	panels := panel.NewManager(parsed.Target)
	panels.ByID("system").Title = localizer.T("系统")
	value := &App{
		config:                config,
		parsed:                parsed,
		ctx:                   ctx,
		cancel:                cancel,
		events:                make(chan interface{}, 256),
		panels:                panels,
		monitorLast:           make(map[string]time.Time),
		monitorActive:         make(map[string]bool),
		panelScroll:           make(map[string]int),
		monitorSlots:          make(map[monitorLayoutMode]map[int]string),
		monitorSlot:           -1,
		focus:                 focusTerminal,
		aiExpanded:            true,
		lineReliable:          true,
		outputTail:            newRollingBuffer(16 * 1024),
		recentOutput:          newContextBuffer(config.AIContextChars),
		agent:                 provider,
		localizer:             localizer,
		historyConversationID: history.NewConversationID(),
		dirty:                 true,
	}
	if strings.TrimSpace(config.HistoryDir) != "" {
		value.historyStore = history.New(config.HistoryDir)
		value.historyTarget = parsed.Target
		value.loadChatHistory()
	}
	value.appendAIReadyMessages()
	return value, nil
}

func (a *App) t(message string, args ...any) string {
	return a.localizer.T(message, args...)
}

func newAgentProvider(config Config) (agent.Provider, error) {
	mode := agentMode(config)
	switch mode {
	case "codex", "local":
		provider := agent.NewCodexProvider(config.CodexPath, config.CodexModel)
		provider.Language = config.Language
		provider.SetLocalWorkspaceWritable(config.WorkspaceWritable)
		provider.SetLocalWorkspace(config.Workspace)
		provider.Timeout = config.AITimeout
		provider.OutputContextChars = config.AIContextChars
		return provider, nil
	case "claude":
		provider := agent.NewClaudeProvider(config.ClaudePath, config.ClaudeModel)
		provider.Language = config.Language
		provider.SetLocalWorkspaceWritable(config.WorkspaceWritable)
		provider.SetLocalWorkspace(config.Workspace)
		provider.Timeout = config.AITimeout
		provider.OutputContextChars = config.AIContextChars
		return provider, nil
	case "http", "remote":
		provider := agent.NewClient(config.AgentURL, config.AgentToken)
		provider.HTTP.Timeout = config.AITimeout
		return provider, nil
	case "deepseek":
		provider := agent.NewDirectModelProvider("DeepSeek", config.ModelURL, config.ModelAPIKey, config.ModelName, true)
		provider.Language = config.Language
		provider.HTTP.Timeout = config.AITimeout
		provider.OutputContextChars = config.AIContextChars
		return provider, nil
	case "model", "openai-compatible":
		provider := agent.NewDirectModelProvider("Model", config.ModelURL, config.ModelAPIKey, config.ModelName, false)
		provider.Language = config.Language
		provider.HTTP.Timeout = config.AITimeout
		provider.OutputContextChars = config.AIContextChars
		return provider, nil
	default:
		return nil, fmt.Errorf("unsupported AI provider %q (use codex, claude, deepseek, model, or http)", mode)
	}
}

func agentMode(config Config) string {
	mode := strings.ToLower(strings.TrimSpace(config.AgentProvider))
	if mode == "" {
		if strings.TrimSpace(config.AgentURL) != "" {
			mode = "http"
		} else {
			mode = "codex"
		}
	}
	return mode
}

func resolveLocalWorkspace(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func (a *App) Run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("aissh must run in an interactive terminal")
	}
	if err := a.updateSize(); err != nil {
		return err
	}
	if err := a.startSSH(); err != nil {
		return err
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		a.stopSSH()
		return err
	}
	defer func() {
		a.cancel()
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
		_, _ = io.WriteString(os.Stdout, "\x1b[?2026l"+bracketedPasteDisable+mouseReportingDisable+"\x1b[0m\x1b[?25h\x1b[?1049l")
		a.stopSSH()
		if closer, ok := a.agent.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		if a.controlDir != "" {
			_ = os.RemoveAll(a.controlDir)
		}
	}()
	_, _ = io.WriteString(os.Stdout, "\x1b[?1049h\x1b[2J\x1b[H"+bracketedPasteEnable+mouseReportingDisable)

	a.startInputReader()
	a.startSignalReader()
	a.startOutputReader()

	maintenanceTicker := time.NewTicker(120 * time.Millisecond)
	frameTicker := time.NewTicker(terminalFrameInterval)
	defer maintenanceTicker.Stop()
	defer frameTicker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return nil
		case event := <-a.events:
			a.handleEvent(event)
		case <-maintenanceTicker.C:
			now := time.Now()
			a.checkZmodemCompletion(now)
			a.maintainAIKeepalive(now)
			a.checkInBandMonitorTimeout(now)
			a.scheduleMonitors()
			if a.inlineAI != nil && a.agentBusy {
				a.inlineAI.Spinner++
				a.dirty = true
			}
		case <-frameTicker.C:
			if a.dirty && a.zmodem == nil {
				a.render()
				a.dirty = false
			}
		}
		if a.terminalDone {
			if a.dirty {
				a.render()
				a.dirty = false
			}
			return nil
		}
	}
}

func (a *App) startSSH() error {
	args := append([]string(nil), a.config.SSHArgs...)
	if a.config.SSHKeepalive > 0 {
		args = append(sshServerAliveOptions(a.config.SSHKeepalive), args...)
	}
	if supportsSSHControlPath() && !sshwrap.HasControlPath(args) {
		directory, err := os.MkdirTemp("", "aissh-")
		if err != nil {
			return err
		}
		if err := os.Chmod(directory, 0700); err != nil {
			return err
		}
		a.controlDir = directory
		a.controlPath = filepath.Join(directory, "master")
		args = append(sshwrap.ControlOptions(a.controlPath), args...)
	}
	args = interactiveSSHArgs(args)
	cmd := exec.CommandContext(a.ctx, a.config.SSHPath, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := startSSHSession(cmd, a.contentRows, a.mainCols)
	if err != nil {
		return err
	}
	a.sshCommand = cmd
	a.sshPTY = ptmx
	a.virtual = vt10x.New(vt10x.WithWriter(ptmx), vt10x.WithSize(a.mainCols, a.contentRows))
	a.monitor = monitor.Runner{
		SSHPath:     a.config.SSHPath,
		Parsed:      a.parsed,
		ControlPath: a.controlPath,
		Timeout:     8 * time.Second,
		Language:    a.localizer.Language(),
	}
	// Win32-OpenSSH does not implement ControlMaster/ControlPath. Running
	// monitor queries as separate SSH processes would therefore require a
	// second non-interactive authentication and fails for password-based
	// sessions. Reuse the already authenticated interactive shell instead.
	a.monitorViaPrimary = !supportsSSHControlPath()
	go func() { a.events <- sshExitEvent{err: waitSSHProcess(a.ctx, cmd)} }()
	return nil
}

func (a *App) stopSSH() {
	if a.sshCommand != nil && a.sshCommand.Process != nil {
		stopSSHProcess(a.sshCommand.Process)
	}
	if a.sshPTY != nil {
		_ = a.sshPTY.Close()
	}
}

func (a *App) startOutputReader() {
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			count, err := a.sshPTY.Read(buffer)
			if count > 0 {
				a.events <- outputEvent{data: append([]byte(nil), buffer[:count]...)}
			}
			if err != nil {
				if !errors.Is(err, os.ErrClosed) {
					a.events <- sshExitEvent{err: err}
				}
				return
			}
		}
	}()
}

func (a *App) writeTerminalOutput(tokens []terminalview.OutputToken) []terminalview.Line {
	var history []terminalview.Line
	for index := 0; index < len(tokens); {
		token := tokens[index]
		switch token.Kind {
		case terminalview.OutputText:
			cells := terminalview.CellsUntilAutoWrap(a.virtual, a.mainCols)
			if cells == 0 {
				if line, ok := terminalview.CaptureTopLineBeforeToken(a.virtual, a.contentRows-1, a.mainCols, token); ok {
					history = append(history, line)
				}
				// The first token consumes the pending wrap, then the rest can fill
				// the newly entered row without causing another wrap.
				cells = a.mainCols
			}
			end := index
			for end < len(tokens) && end-index < cells && tokens[end].Kind == terminalview.OutputText {
				end++
			}
			_, _ = a.virtual.Write(joinOutputTokens(tokens[index:end]))
			index = end
		case terminalview.OutputLineFeed:
			if line, ok := terminalview.CaptureTopLineBeforeToken(a.virtual, a.contentRows-1, a.mainCols, token); ok {
				history = append(history, line)
			}
			_, _ = a.virtual.Write(token.Data)
			index++
		default:
			end := index + 1
			for end < len(tokens) && tokens[end].Kind == terminalview.OutputControl {
				end++
			}
			_, _ = a.virtual.Write(joinOutputTokens(tokens[index:end]))
			index = end
		}
	}
	return history
}

func joinOutputTokens(tokens []terminalview.OutputToken) []byte {
	if len(tokens) == 1 {
		return tokens[0].Data
	}
	size := 0
	for _, token := range tokens {
		size += len(token.Data)
	}
	data := make([]byte, 0, size)
	for _, token := range tokens {
		data = append(data, token.Data...)
	}
	return data
}

func (a *App) startInputReader() {
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := os.Stdin.Read(buffer)
			if count > 0 {
				a.events <- inputEvent{data: append([]byte(nil), buffer[:count]...)}
			}
			if err != nil {
				return
			}
		}
	}()
}

func (a *App) updateSize() error {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	if cols < 40 || rows < 10 {
		return fmt.Errorf("terminal is too small; need at least 40x10")
	}
	a.setViewportSize(cols, rows)
	return nil
}

func (a *App) setViewportSize(cols, rows int) {
	a.cols, a.rows = cols, rows
	a.contentRows = rows - 2
	a.compact = cols < 92
	if !a.compact && a.sidebarCollapsed && a.sidebarWorkspaceFocused() {
		a.sidebarCollapsed = false
	}
	a.applyPaneLayout()
}

func (a *App) applyPaneLayout() {
	if a.cols <= 0 || a.contentRows <= 0 {
		return
	}
	previousMainCols := a.mainCols
	// mainCols is the SSH content width; the column right of it (mainCols+1) is
	// reserved as the scrollbar gutter so the scrollbar never covers pane text.
	if a.compact || a.sidebarCollapsed {
		a.mainCols = a.cols - 1
	} else {
		a.mainCols = a.cols*64/100 - 1
		if a.mainCols < 48 {
			a.mainCols = 48
		}
		if a.cols-a.mainCols-2 < 32 {
			a.mainCols = a.cols - 34
		}
	}
	if previousMainCols != a.mainCols {
		a.terminalRows = nil
	}
	if a.virtual != nil {
		a.virtual.Resize(a.mainCols, a.contentRows)
	}
	if a.sshPTY != nil {
		resizeSSHSession(a.sshPTY, a.contentRows, a.mainCols)
	}
}

func (a *App) sidebarVisible() bool {
	return !a.compact && !a.sidebarCollapsed
}

func (a *App) sidebarWorkspaceFocused() bool {
	switch a.focus {
	case focusAI, focusAIHistory, focusMonitor, focusMonitorSlot, focusPanelMenu, focusCustomPanel:
		return true
	default:
		return false
	}
}

func (a *App) toggleSidebar() {
	if a.compact {
		return
	}
	a.sidebarCollapsed = !a.sidebarCollapsed
	if a.sidebarCollapsed {
		a.focus = focusTerminal
	}
	a.applyPaneLayout()
	a.dirty = true
}

func (a *App) ensureSidebarVisible() {
	if a.compact || !a.sidebarCollapsed {
		return
	}
	a.sidebarCollapsed = false
	a.applyPaneLayout()
	a.dirty = true
}

func (a *App) handleEvent(event interface{}) {
	switch value := event.(type) {
	case inputEvent:
		a.handleInput(value.data)
	case outputEvent:
		a.processSSHOutput(value.data)
	case resizeEvent:
		_ = a.updateSize()
		a.dirty = true
	case sshExitEvent:
		if !a.terminalDone && value.err != nil && !strings.Contains(value.err.Error(), "input/output error") {
			a.appendPersistentChat(chatMessage{Role: "system", Content: a.t("SSH 已结束：%s", value.err.Error()), Time: time.Now()})
		}
		a.terminalDone = true
		a.dirty = true
	case monitorResultEvent:
		a.applyMonitorResult(value)
	case agentStatusEvent:
		a.agentStatus = a.t(value.status)
		if a.agentInline != nil {
			if a.agentInline.StatusPrefix != "" {
				a.agentInline.Status = a.agentInline.StatusPrefix + " · " + a.t(value.status)
			} else {
				a.agentInline.Status = a.t(value.status)
			}
		}
		a.dirty = true
	case agentThoughtEvent:
		a.agentThought += value.content
		if len(a.agentThought) > 12000 {
			a.agentThought = a.agentThought[len(a.agentThought)-12000:]
		}
		a.dirty = true
	case agentAnswerEvent:
		a.agentThought = ""
		a.appendAssistantDelta(value.content)
		if a.agentInline != nil {
			a.agentInline.Response += value.content
		}
		a.dirty = true
	case agentTurnEvent:
		a.finishAgentTurn(value.turn, value.err)
	case agentPanelEvent:
		a.applyPanelAction(value.action)
	case approvalEvent:
		a.finishApproval(value)
	case rejectionEvent:
		a.finishRejection(value)
	case executionResultEvent:
		a.finishExecutionResult(value)
	case delayedErrorTriggerEvent:
		if a.pendingError != nil && a.pendingError.Kind == value.match.Kind {
			a.pendingError = nil
			a.startAI(a.t("刚才命令 %q 执行失败：%s。请分析原因并给出安全的下一步。", value.command, value.match.Message), true)
		}
	}
}

func (a *App) monitorWorkspaceVisible() bool {
	if a.sidebarVisible() {
		return true
	}
	if !a.compact {
		return false
	}
	return a.focus == focusMonitor || a.focus == focusMonitorSlot || a.focus == focusPanelMenu || a.focus == focusCustomPanel
}

func (a *App) handleOutput(data []byte) {
	a.observeTerminalClear(data)
	plain := trigger.StripANSI(string(data))
	a.outputTail.Append(plain)
	a.recentOutput.Append(plain)
	if a.execution != nil && a.execution.Output != nil {
		a.execution.Output.Append(plain)
	}
	a.sensitive = trigger.LooksSensitive(a.outputTail.String())
	errorWindow := a.errorCarry + plain
	if match := trigger.MatchError(errorWindow); match != nil {
		a.pendingError = match
	}
	if len(errorWindow) > 512 {
		errorWindow = errorWindow[len(errorWindow)-512:]
		for len(errorWindow) > 0 && (errorWindow[0]&0xc0) == 0x80 {
			errorWindow = errorWindow[1:]
		}
	}
	a.errorCarry = errorWindow
	if trigger.LooksLikePrompt(a.outputTail.String()) {
		if !a.atPrompt {
			a.promptIdleSince = time.Now()
		}
		a.atPrompt = true
		a.sensitive = false
		if a.execution != nil {
			a.submitExecutionResult()
		} else if a.pendingError != nil && a.pendingError.Kind == "command_not_found" && !a.agentBusy {
			match := *a.pendingError
			command := a.lastCommand
			go func() {
				time.Sleep(120 * time.Millisecond)
				a.events <- delayedErrorTriggerEvent{match: match, command: command, createdAt: time.Now()}
			}()
		}
	}
	a.dirty = true
}

func (a *App) scheduleMonitors() {
	now := time.Now()
	if a.monitorViaPrimary {
		a.scheduleInBandMonitor(now)
		return
	}
	for _, item := range a.panels.Panels() {
		if item.Kind != panel.KindMonitor && item.Kind != panel.KindTable && item.Kind != panel.KindLog {
			continue
		}
		if item.Command == "" || a.monitorActive[item.ID] {
			continue
		}
		interval := item.Interval
		if interval < time.Second {
			interval = time.Second
		}
		if last := a.monitorLast[item.ID]; !last.IsZero() && now.Sub(last) < interval {
			continue
		}
		if !policy.IsReadOnlyMonitorCommand(item.Command) {
			a.panels.SetError(item.ID, fmt.Errorf("%s", a.t("监控命令被只读策略拒绝")))
			a.monitorLast[item.ID] = now
			continue
		}
		a.monitorActive[item.ID] = true
		panelID, command := item.ID, item.Command
		go func() {
			content, err := a.monitor.Run(a.ctx, command)
			a.events <- monitorResultEvent{panelID: panelID, content: content, err: err}
		}()
	}
}

func (a *App) applyMonitorResult(value monitorResultEvent) {
	a.monitorActive[value.panelID] = false
	a.monitorLast[value.panelID] = time.Now()
	if value.err != nil {
		a.panels.SetError(value.panelID, value.err)
	} else {
		a.panels.UpdateContent(value.panelID, value.content, a.t("实时"), false)
	}
	if a.monitorWorkspaceVisible() {
		a.dirty = true
	}
}

func bytesToRunes(data []byte) []rune {
	if utf8.Valid(data) {
		return []rune(string(data))
	}
	return []rune(string([]byte{data[0]}))
}
