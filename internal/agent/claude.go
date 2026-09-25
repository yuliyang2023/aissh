package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/i18n"
)

// claudeReadOnlyTools is the tool allowlist for local workspace analysis; it
// must stay read-only so aissh remains the only writer on either machine.
const claudeReadOnlyTools = "Read,Grep,Glob"

// claudeWritableTools adds only file-editing tools. Bash and network tools stay
// disabled because Claude Code does not provide the same workspace-scoped shell
// sandbox as Codex.
const claudeWritableTools = claudeReadOnlyTools + ",Edit,Write,NotebookEdit"

// claudeDisallowedTools blocks every tool that could mutate local state or
// reach the network; Bash is included because it cannot be proven read-only.
const claudeDisallowedTools = "Bash,Edit,Write,NotebookEdit,WebFetch,WebSearch,Task,TodoWrite"

const claudeWritableDisallowedTools = "Bash,WebFetch,WebSearch,Task,TodoWrite"

type claudeRunner func(context.Context, string, []string, string, string) ([]byte, []byte, error)

type claudeResult struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	SessionID        string          `json:"session_id"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

// ClaudeProvider invokes the locally installed Claude Code CLI in print mode.
// Each provider keeps a persistent Claude session for its aissh SSH session and
// only returns structured advice. Local workspace writes are disabled by
// default and must be explicitly enabled; aissh remains solely responsible for
// executing approved commands over SSH.
type ClaudeProvider struct {
	Path               string
	Model              string
	Timeout            time.Duration
	Language           string
	Workspace          string
	WorkspaceWritable  bool
	OutputContextChars int

	mu       sync.Mutex
	active   sync.WaitGroup
	closed   bool
	sessions map[string]*codexSession
	run      claudeRunner
	lookPath func(string) (string, error)
}

func NewClaudeProvider(path, model string) *ClaudeProvider {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "claude"
	}
	return &ClaudeProvider{
		Path:               path,
		Model:              strings.TrimSpace(model),
		Timeout:            90 * time.Second,
		OutputContextChars: DefaultOutputContextChars,
		sessions:           make(map[string]*codexSession),
		run:                runClaude,
		lookPath:           exec.LookPath,
	}
}

func (p *ClaudeProvider) Name() string { return "Claude" }

func (p *ClaudeProvider) localizer() i18n.Localizer { return i18n.New(p.Language) }

// SetLocalWorkspace changes the local directory used by subsequent Claude
// turns. An empty value disables local workspace access.
func (p *ClaudeProvider) SetLocalWorkspace(value string) {
	p.mu.Lock()
	workspace := strings.TrimSpace(value)
	if p.Workspace != workspace {
		p.sessions = make(map[string]*codexSession)
	}
	p.Workspace = workspace
	p.mu.Unlock()
}

// SetLocalWorkspaceWritable controls whether subsequent Claude sessions can
// use file-editing tools. Changing the mode starts a fresh local AI session.
func (p *ClaudeProvider) SetLocalWorkspaceWritable(value bool) {
	p.mu.Lock()
	if p.WorkspaceWritable != value {
		p.sessions = make(map[string]*codexSession)
	}
	p.WorkspaceWritable = value
	p.mu.Unlock()
}

func (p *ClaudeProvider) LocalWorkspace() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.TrimSpace(p.Workspace)
}

func (p *ClaudeProvider) localWorkspaceSettings() (string, bool) {
	if p == nil {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.TrimSpace(p.Workspace), p.WorkspaceWritable
}

func (p *ClaudeProvider) Configured() bool {
	if p == nil || strings.TrimSpace(p.Path) == "" {
		return false
	}
	_, err := p.lookPath(p.Path)
	return err == nil
}

func (p *ClaudeProvider) Stream(ctx context.Context, request Request, callbacks Callbacks) (Turn, error) {
	if !p.Configured() {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("找不到本机 Claude CLI；请先安装 Claude Code 或设置 AISSH_CLAUDE_PATH"))
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Claude 会话已关闭"))
	}
	p.active.Add(1)
	p.mu.Unlock()
	defer p.active.Done()

	session, created, err := p.session(request)
	if err != nil {
		return Turn{}, err
	}
	input := p.requestText(session, request)
	request.SystemContext = cloneContext(request.SystemContext)
	workspace, workspaceWritable := p.localWorkspaceSettings()
	if workspace != "" {
		request.SystemContext["local_workspace"] = workspace
		if workspaceWritable {
			request.SystemContext["local_workspace_mode"] = "read-write"
		}
	}
	p.mu.Lock()
	history := append([]codexHistory(nil), session.History...)
	pending := append([]string(nil), session.Pending...)
	sessionID := session.ID
	responseLanguage := strings.TrimSpace(request.SystemContext["response_language"])
	if responseLanguage == "" {
		if request.Event == "result" && session.ResponseLanguage != "" {
			responseLanguage = session.ResponseLanguage
		} else {
			responseLanguage = i18n.PreferredResponseLanguage(request.Prompt, firstNonEmpty(p.Language, request.SystemContext["ui_language"]))
		}
	}
	responseLanguage = i18n.Resolve(responseLanguage)
	session.ResponseLanguage = responseLanguage
	p.mu.Unlock()
	request.SystemContext["response_language"] = responseLanguage
	if !created && len(pending) > 0 {
		input = strings.Join(pending, "\n") + "\n\n" + input
	}

	if callbacks.OnStatus != nil {
		callbacks.OnStatus(p.localizer().T("正在调用本机 Claude"))
	}
	prompt := buildCodexPrompt(request, history, input)
	if !created {
		prompt = buildCodexContinuationPrompt(request, input)
	}
	result, returnedSessionID, err := p.invoke(ctx, prompt, workspace, workspaceWritable, sessionID)
	if err != nil && !created && codexSessionMissing(err) {
		p.mu.Lock()
		delete(p.sessions, sessionID)
		p.mu.Unlock()
		prompt = buildCodexPrompt(request, history, input)
		result, returnedSessionID, err = p.invoke(ctx, prompt, workspace, workspaceWritable, "")
		created = true
	}
	if err != nil {
		return Turn{}, err
	}
	returnedSessionID = strings.TrimSpace(returnedSessionID)
	if created && returnedSessionID == "" {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Claude 没有返回会话 ID"))
	}
	p.mu.Lock()
	if returnedSessionID != "" && returnedSessionID != session.ID {
		delete(p.sessions, session.ID)
		session.ID = returnedSessionID
	}
	if created {
		// One aissh SSH session keeps exactly one live Claude session.
		p.sessions = map[string]*codexSession{session.ID: session}
	} else {
		p.sessions[session.ID] = session
	}
	p.mu.Unlock()

	if result.Type == "command_proposal" && (result.Proposal == nil || strings.TrimSpace(result.Proposal.Command) == "") {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Claude 返回了空命令建议"))
	}
	if result.Proposal != nil {
		result.Proposal.Command = strings.TrimSpace(result.Proposal.Command)
		if result.Proposal.ID == "" {
			result.Proposal.ID = newLocalID("proposal")
		}
	}
	for _, action := range result.PanelActions {
		if callbacks.OnPanelAction != nil {
			callbacks.OnPanelAction(action)
		}
	}

	p.mu.Lock()
	session.History = append(session.History,
		codexHistory{Role: "user", Content: input},
		codexHistory{Role: "assistant", Content: summarizeCodexResponse(result)},
	)
	if len(session.History) > 16 {
		session.History = append([]codexHistory(nil), session.History[len(session.History)-16:]...)
	}
	if result.Proposal != nil {
		session.Proposals[result.Proposal.ID] = *result.Proposal
	}
	if request.Event == "result" {
		delete(session.Proposals, request.ProposalID)
	}
	session.Pending = nil
	p.mu.Unlock()

	status := "finished"
	if result.Proposal != nil {
		status = "waiting_approval"
	}
	return Turn{
		Type:     firstNonEmpty(result.Type, "finished"),
		Session:  Session{ID: session.ID, Status: status},
		Proposal: result.Proposal,
		Message:  result.Message,
	}, nil
}

// AssessRisk uses a fresh, non-resumed Claude invocation and does not touch the
// provider's main conversation bookkeeping.
func (p *ClaudeProvider) AssessRisk(ctx context.Context, request RiskAssessmentRequest) (RiskAssessment, error) {
	if !p.Configured() {
		return RiskAssessment{}, fmt.Errorf("%s", p.localizer().T("找不到本机 Claude CLI；请先安装 Claude Code 或设置 AISSH_CLAUDE_PATH"))
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return RiskAssessment{}, fmt.Errorf("%s", p.localizer().T("本机 Claude 会话已关闭"))
	}
	p.active.Add(1)
	p.mu.Unlock()
	defer p.active.Done()

	result, _, err := p.invoke(ctx, buildRiskAssessmentPrompt(request), "", false, "")
	if err != nil {
		return RiskAssessment{}, err
	}
	return riskAssessmentFromResponse(result, request.Proposal.Command)
}

// Close drops the in-memory session state. Claude Code has no CLI command to
// delete a stored session, so its local transcript is simply left behind.
func (p *ClaudeProvider) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()
	p.active.Wait()

	p.mu.Lock()
	p.sessions = make(map[string]*codexSession)
	p.mu.Unlock()
	return nil
}

func (p *ClaudeProvider) Action(_ context.Context, request Request) (Turn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Claude 会话已关闭"))
	}
	session := p.sessions[request.SessionID]
	if session == nil {
		return Turn{}, fmt.Errorf("session not found")
	}
	proposal, ok := session.Proposals[request.ProposalID]
	if !ok {
		return Turn{}, fmt.Errorf("proposal not found")
	}
	switch request.Event {
	case "approve":
		message := "用户批准执行命令：" + proposal.Command
		session.History = append(session.History, codexHistory{Role: "user", Content: message})
		session.Pending = append(session.Pending, message)
		return Turn{Type: "approved", Session: Session{ID: session.ID, Status: "running"}}, nil
	case "reject":
		delete(session.Proposals, request.ProposalID)
		message := "用户拒绝执行命令：" + proposal.Command
		session.History = append(session.History, codexHistory{Role: "user", Content: message})
		session.Pending = append(session.Pending, message)
		return Turn{
			Type:    "finished",
			Session: Session{ID: session.ID, Status: "finished"},
			Message: p.localizer().T("已拒绝该命令建议。你可以继续描述希望采用的处理方式。"),
		}, nil
	default:
		return Turn{}, fmt.Errorf("unsupported action: %s", request.Event)
	}
}

func (p *ClaudeProvider) session(request Request) (*codexSession, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if request.Event == "start" || request.SessionID == "" {
		return &codexSession{Proposals: make(map[string]Proposal)}, true, nil
	}
	value := p.sessions[request.SessionID]
	if value == nil {
		return nil, false, fmt.Errorf("session not found")
	}
	return value, false, nil
}

func (p *ClaudeProvider) requestText(session *codexSession, request Request) string {
	switch request.Event {
	case "result":
		command := ""
		p.mu.Lock()
		if proposal, ok := session.Proposals[request.ProposalID]; ok {
			command = proposal.Command
		}
		p.mu.Unlock()
		return fmt.Sprintf("远端命令执行结果\n命令：%s\n完成原因：%s\n输出：\n%s", firstNonEmpty(command, "未知"), request.Completion, LimitOutputContext(request.Output, p.OutputContextChars))
	default:
		parts := []string{"用户请求：" + trimText(request.Prompt, 8000)}
		if strings.TrimSpace(request.RecentOutput) != "" {
			parts = append(parts, "SSH 最近输出：\n"+LimitOutputContext(request.RecentOutput, p.OutputContextChars))
		}
		return strings.Join(parts, "\n\n")
	}
}

func (p *ClaudeProvider) invoke(parent context.Context, prompt, workspace string, workspaceWritable bool, sessionID string) (codexResponse, string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Without a workspace no local file access is allowed at all, so the
	// read-only tools join the disallowed list instead of the allowlist.
	disallowedTools := claudeDisallowedTools
	allowedTools := claudeReadOnlyTools
	if strings.TrimSpace(workspace) == "" {
		disallowedTools += "," + claudeReadOnlyTools
		allowedTools = ""
	} else if workspaceWritable {
		disallowedTools = claudeWritableDisallowedTools
		allowedTools = claudeWritableTools
	}
	args := []string{
		"-p",
		"--output-format", "json",
		"--json-schema", codexResponseSchema,
		"--permission-mode", "dontAsk",
		"--strict-mcp-config",
		"--disallowedTools", disallowedTools,
	}
	if allowedTools != "" {
		args = append(args, "--tools", allowedTools, "--allowedTools", allowedTools)
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if strings.TrimSpace(sessionID) != "" {
		args = append(args, "--resume", strings.TrimSpace(sessionID))
	}
	stdout, stderr, runErr := p.run(ctx, p.Path, args, claudeWorkingDirectory(workspace), prompt)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return codexResponse{}, "", fmt.Errorf("%s", p.localizer().T("本机 Claude 调用超时（%s）", timeout))
	}
	response, returnedSessionID, parseErr := parseClaudeInvocationJSONLocalized(stdout, p.localizer())
	if runErr != nil {
		return codexResponse{}, returnedSessionID, claudeRunErrorLocalized(runErr, stdout, stderr, p.localizer())
	}
	if parseErr != nil {
		return codexResponse{}, returnedSessionID, parseErr
	}
	return response, returnedSessionID, nil
}

func claudeWorkingDirectory(workspace string) string {
	if value := strings.TrimSpace(workspace); value != "" {
		return value
	}
	return os.TempDir()
}

func runClaude(ctx context.Context, path string, args []string, dir, prompt string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = localProviderEnvironment("claude", path)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func parseClaudeInvocationJSONLocalized(data []byte, localizer i18n.Localizer) (codexResponse, string, error) {
	result, found := decodeClaudeResult(data)
	if !found {
		return codexResponse{}, "", fmt.Errorf("%s", localizer.T("本机 Claude 没有返回结果%s", optionalDetail(strings.TrimSpace(string(data)))))
	}
	sessionID := strings.TrimSpace(result.SessionID)
	if result.IsError || (result.Subtype != "" && result.Subtype != "success") {
		return codexResponse{}, sessionID, fmt.Errorf("%s", localizer.T("本机 Claude 调用失败: %s", trimText(firstNonEmpty(result.Result, result.Subtype), 800)))
	}
	structured := strings.TrimSpace(string(result.StructuredOutput))
	payload := structured
	if payload == "" || payload == "null" {
		payload = stripJSONFence(result.Result)
	}
	if strings.TrimSpace(payload) == "" {
		return codexResponse{}, sessionID, fmt.Errorf("%s", localizer.T("本机 Claude 没有返回结果%s", ""))
	}
	var response codexResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		return codexResponse{}, sessionID, fmt.Errorf("%s", localizer.T("无法解析本机 Claude 的结构化结果: %v", err))
	}
	return response, sessionID, nil
}

// decodeClaudeResult parses the single JSON document that
// `--output-format json` emits.
func decodeClaudeResult(data []byte) (claudeResult, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return claudeResult{}, false
	}
	var result claudeResult
	if err := json.Unmarshal(trimmed, &result); err != nil {
		return claudeResult{}, false
	}
	if result.SessionID == "" && result.Result == "" && len(result.StructuredOutput) == 0 {
		return claudeResult{}, false
	}
	return result, true
}

func claudeRunErrorLocalized(runErr error, stdout, stderr []byte, localizer i18n.Localizer) error {
	detail := string(append(append([]byte(nil), stderr...), stdout...))
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "invalid api key") || strings.Contains(lower, "please run /login") || strings.Contains(lower, "oauth token has expired") || strings.Contains(lower, "401 unauthorized") || strings.Contains(lower, "authentication required") {
		return fmt.Errorf("%s", localizer.T("本机 Claude 认证失败；请先确认 `claude -p` 可用并已登录，或设置 ANTHROPIC_API_KEY"))
	}
	if strings.Contains(lower, "no conversation found") || strings.Contains(lower, "session not found") || strings.Contains(lower, "unknown session") {
		return fmt.Errorf("session not found: %s", localizer.T("本机 Claude 会话已失效"))
	}
	clean := strings.TrimSpace(detail)
	if clean == "" {
		clean = runErr.Error()
	}
	return fmt.Errorf("%s", localizer.T("本机 Claude 调用失败: %s", trimText(clean, 800)))
}
