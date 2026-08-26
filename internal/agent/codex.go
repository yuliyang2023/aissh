package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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

const codexResponseSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "type": {
      "type": "string",
      "enum": ["finished", "command_proposal", "ask_user", "failed"]
    },
    "message": {"type": "string"},
    "proposal": {
      "anyOf": [
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "id": {"type": "string"},
            "command": {"type": "string"},
            "explanation": {"type": "string"},
            "expectedEffect": {"type": "string"},
            "risk": {
              "type": "string",
              "enum": ["read_only", "low", "medium", "high", "critical"]
            }
          },
          "required": ["id", "command", "explanation", "expectedEffect", "risk"]
        },
        {"type": "null"}
      ]
    },
    "panelActions": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "action": {"type": "string", "enum": ["open", "create", "update", "append", "close"]},
          "panel": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
              "id": {"type": "string"},
              "type": {"type": "string", "enum": ["monitor", "log"]},
              "title": {"type": "string"},
              "collector": {"type": "string"},
              "command": {"type": "string"},
              "intervalMs": {"type": "integer", "minimum": 0},
              "content": {"type": "string"},
              "allowAIRead": {"type": "boolean"}
            },
            "required": ["id", "type", "title", "collector", "command", "intervalMs", "content", "allowAIRead"]
          }
        },
        "required": ["action", "panel"]
      }
    }
  },
  "required": ["type", "message", "proposal", "panelActions"]
}`

type codexResponse struct {
	Type         string        `json:"type"`
	Message      string        `json:"message"`
	Proposal     *Proposal     `json:"proposal"`
	PanelActions []PanelAction `json:"panelActions"`
}

type codexHistory struct {
	Role    string
	Content string
}

type codexSession struct {
	ID               string
	History          []codexHistory
	Pending          []string
	Proposals        map[string]Proposal
	ResponseLanguage string
}

type codexRunner func(context.Context, string, []string, string) ([]byte, []byte, error)

// CodexProvider invokes the locally installed Codex CLI. Each provider keeps a
// persistent, read-only Codex thread for its aissh SSH session and only returns
// structured advice; aissh remains solely responsible for executing approved
// commands over SSH.
type CodexProvider struct {
	Path               string
	Model              string
	Timeout            time.Duration
	Language           string
	Workspace          string
	OutputContextChars int

	mu       sync.Mutex
	active   sync.WaitGroup
	closed   bool
	sessions map[string]*codexSession
	owned    map[string]struct{}
	run      codexRunner
	lookPath func(string) (string, error)
}

func NewCodexProvider(path, model string) *CodexProvider {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "codex"
	}
	return &CodexProvider{
		Path:               path,
		Model:              strings.TrimSpace(model),
		Timeout:            90 * time.Second,
		OutputContextChars: DefaultOutputContextChars,
		sessions:           make(map[string]*codexSession),
		owned:              make(map[string]struct{}),
		run:                runCodex,
		lookPath:           exec.LookPath,
	}
}

func (p *CodexProvider) Name() string { return "Codex" }

func (p *CodexProvider) localizer() i18n.Localizer { return i18n.New(p.Language) }

// SetLocalWorkspace changes the read-only local directory used by subsequent
// Codex turns. An empty value disables local workspace access.
func (p *CodexProvider) SetLocalWorkspace(value string) {
	p.mu.Lock()
	workspace := strings.TrimSpace(value)
	if p.Workspace != workspace {
		p.sessions = make(map[string]*codexSession)
	}
	p.Workspace = workspace
	p.mu.Unlock()
}

func (p *CodexProvider) LocalWorkspace() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.TrimSpace(p.Workspace)
}

func (p *CodexProvider) Configured() bool {
	if p == nil || strings.TrimSpace(p.Path) == "" {
		return false
	}
	_, err := p.lookPath(p.Path)
	return err == nil
}

func (p *CodexProvider) Stream(ctx context.Context, request Request, callbacks Callbacks) (Turn, error) {
	if !p.Configured() {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("找不到本机 Codex CLI；请先安装 Codex 或设置 AISSH_CODEX_PATH"))
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 会话已关闭"))
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
	workspace := p.LocalWorkspace()
	if workspace != "" {
		request.SystemContext["local_workspace"] = workspace
	}
	p.mu.Lock()
	history := append([]codexHistory(nil), session.History...)
	pending := append([]string(nil), session.Pending...)
	threadID := session.ID
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
		callbacks.OnStatus(p.localizer().T("正在调用本机 Codex"))
	}
	prompt := buildCodexPrompt(request, history, input)
	if !created {
		prompt = buildCodexContinuationPrompt(request, input)
	}
	result, resumedThreadID, err := p.invoke(ctx, prompt, workspace, threadID)
	if err != nil && !created && codexSessionMissing(err) {
		p.mu.Lock()
		delete(p.sessions, threadID)
		delete(p.owned, threadID)
		p.mu.Unlock()
		prompt = buildCodexPrompt(request, history, input)
		result, resumedThreadID, err = p.invoke(ctx, prompt, workspace, "")
		created = true
	}
	if err != nil {
		return Turn{}, err
	}
	if created {
		if strings.TrimSpace(resumedThreadID) == "" {
			return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 没有返回会话 ID"))
		}
		var staleThreadIDs []string
		p.mu.Lock()
		for ownedThreadID := range p.owned {
			if ownedThreadID != resumedThreadID {
				staleThreadIDs = append(staleThreadIDs, ownedThreadID)
				delete(p.sessions, ownedThreadID)
			}
		}
		session.ID = strings.TrimSpace(resumedThreadID)
		p.sessions[session.ID] = session
		p.owned[session.ID] = struct{}{}
		p.mu.Unlock()
		for _, staleThreadID := range staleThreadIDs {
			if p.deleteThread(staleThreadID) == nil {
				p.mu.Lock()
				delete(p.owned, staleThreadID)
				p.mu.Unlock()
			}
		}
	} else if resumedThreadID != "" && resumedThreadID != session.ID {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 恢复了意外的会话：%s", resumedThreadID))
	}

	if result.Type == "command_proposal" && (result.Proposal == nil || strings.TrimSpace(result.Proposal.Command) == "") {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 返回了空命令建议"))
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

// Close removes the persisted Codex threads created for this provider. One
// provider belongs to one aissh SSH process, so its Codex session lifetime
// follows the SSH session lifetime while aissh keeps its own text history.
func (p *CodexProvider) Close() error {
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
	threadIDs := make([]string, 0, len(p.owned))
	for threadID := range p.owned {
		threadIDs = append(threadIDs, threadID)
	}
	p.sessions = make(map[string]*codexSession)
	p.owned = make(map[string]struct{})
	p.mu.Unlock()

	var closeErrors []error
	for _, threadID := range threadIDs {
		if err := p.deleteThread(threadID); err != nil {
			closeErrors = append(closeErrors, err)
		}
	}
	return errors.Join(closeErrors...)
}

func (p *CodexProvider) deleteThread(threadID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, stderr, err := p.run(ctx, p.Path, []string{"delete", "--force", strings.TrimSpace(threadID)}, "")
	if err != nil {
		return codexRunErrorLocalized(err, stdout, stderr, p.localizer())
	}
	return nil
}

func (p *CodexProvider) Action(_ context.Context, request Request) (Turn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return Turn{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 会话已关闭"))
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

func (p *CodexProvider) session(request Request) (*codexSession, bool, error) {
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

func (p *CodexProvider) requestText(session *codexSession, request Request) string {
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

func (p *CodexProvider) invoke(parent context.Context, prompt, workspace, threadID string) (codexResponse, string, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	schema, err := os.CreateTemp("", "aissh-codex-schema-*.json")
	if err != nil {
		return codexResponse{}, "", err
	}
	schemaPath := schema.Name()
	defer os.Remove(schemaPath)
	if _, err := schema.WriteString(codexResponseSchema); err != nil {
		_ = schema.Close()
		return codexResponse{}, "", err
	}
	if err := schema.Close(); err != nil {
		return codexResponse{}, "", err
	}

	args := []string{"exec"}
	if strings.TrimSpace(threadID) != "" {
		args = append(args, "resume")
	}
	args = append(args,
		"--json", "--skip-git-repo-check", "--ignore-rules",
		"--disable", "plugins", "--disable", "remote_plugin", "--disable", "apps",
		"--disable", "browser_use", "--disable", "computer_use", "--disable", "image_generation",
		"--disable", "multi_agent", "--disable", "hooks",
		"--output-schema", schemaPath,
	)
	if strings.TrimSpace(threadID) == "" {
		args = append(args, "--sandbox", "read-only", "-C", codexWorkingDirectory(workspace))
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if strings.TrimSpace(threadID) != "" {
		args = append(args, strings.TrimSpace(threadID))
	}
	args = append(args, "-")
	stdout, stderr, runErr := p.run(ctx, p.Path, args, prompt)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return codexResponse{}, "", fmt.Errorf("%s", p.localizer().T("本机 Codex 调用超时（%s）", timeout))
	}
	response, returnedThreadID, parseErr := parseCodexInvocationJSONLLocalized(stdout, p.localizer())
	if runErr != nil {
		return codexResponse{}, returnedThreadID, codexRunErrorLocalized(runErr, stdout, stderr, p.localizer())
	}
	if parseErr != nil {
		return codexResponse{}, returnedThreadID, parseErr
	}
	return response, returnedThreadID, nil
}

func codexWorkingDirectory(workspace string) string {
	if value := strings.TrimSpace(workspace); value != "" {
		return value
	}
	return os.TempDir()
}

func runCodex(ctx context.Context, path string, args []string, prompt string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func parseCodexJSONL(data []byte) (codexResponse, error) {
	response, _, err := parseCodexInvocationJSONLLocalized(data, i18n.New(""))
	return response, err
}

func parseCodexInvocationJSONLLocalized(data []byte, localizer i18n.Localizer) (codexResponse, string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var assistantText string
	var lastError string
	var threadID string
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Message string `json:"message"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Type == "thread.started" && strings.TrimSpace(event.ThreadID) != "" {
			threadID = strings.TrimSpace(event.ThreadID)
		}
		if event.Type == "error" && event.Message != "" {
			lastError = event.Message
		}
		if event.Type == "item.completed" {
			switch event.Item.Type {
			case "agent_message":
				assistantText = event.Item.Text
			case "error":
				lastError = firstNonEmpty(event.Item.Message, event.Item.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return codexResponse{}, threadID, err
	}
	if strings.TrimSpace(assistantText) == "" {
		return codexResponse{}, threadID, fmt.Errorf("%s", localizer.T("本机 Codex 没有返回结果%s", optionalDetail(lastError)))
	}
	assistantText = stripJSONFence(assistantText)
	var response codexResponse
	if err := json.Unmarshal([]byte(assistantText), &response); err != nil {
		return codexResponse{}, threadID, fmt.Errorf("%s", localizer.T("无法解析本机 Codex 的结构化结果: %v", err))
	}
	return response, threadID, nil
}

func buildCodexPrompt(request Request, history []codexHistory, input string) string {
	var value strings.Builder
	value.WriteString(`你是 aissh 的本地运维助手。aissh 正在本机包装一个真实的 OpenSSH 会话。
`)
	if workspace := strings.TrimSpace(request.SystemContext["local_workspace"]); workspace != "" {
		value.WriteString(`
本机代码上下文：
1. 你可以使用只读本机工具检查工作区中的源码、配置、Git 状态和差异，以便把本地实现与线上现象关联起来。
2. 本机工作区严格只读：不得编辑、创建或删除文件，不得运行可能写文件的构建、测试或项目程序，不得访问网络。
3. 不要把本机工作区命令作为 command_proposal 返回；command_proposal 永远表示将在目标服务器当前 Shell 中执行的远端命令。
`)
	} else {
		value.WriteString(`
本机代码上下文：不要调用任何本机工具、Shell、网络、文件或外部服务；只分析下面提供的文本。
`)
	}
	value.WriteString(`
远端操作硬性边界：
1. 你不能直接操作目标服务器。需要操作时，只能返回一个 command_proposal，命令由用户明确批准后才会经现有 SSH PTY 执行。
2. 命令会在目标服务器当前 SSH 连接的隔离子 Shell 中执行，必须自包含；不得依赖 cd、export、alias、函数或 Shell 选项在命令结束后继续保留。
3. 不得使用 exit、logout，也不得用 exec 替换 Shell；前置条件失败时应输出诊断并用 false 或自然返回结束命令，绝不能主动结束 SSH 登录 Shell。
4. 命令必须适合在目标服务器的当前交互式 Shell 中执行，不得包含交互式全屏程序。
5. 有破坏性、重启、删除、覆盖、权限或网络策略变更风险时，risk 必须如实标为 high 或 critical，并在 explanation 中说明。
6. 可用 panelActions 创建 monitor 或 log Tab，command 必须严格只读；分析结论和报告内容统一写入 message，不创建报告面板；不需要监控面板时返回空数组。
7. 如果信息不足，返回 ask_user 并在 message 中提出一个简短问题；能直接回答时返回 finished。
8. 只返回符合 JSON Schema 的对象，不要 Markdown 代码围栏。

常用只读 collector：system.overview、process.top、disk.usage、network.sockets、service.status。
`)
	if strings.HasPrefix(strings.ToLower(request.SystemContext["response_language"]), "en") {
		value.WriteString("\n输出语言：本轮用户使用英文提问，因此 message、proposal explanation、expectedEffect 和 panel 标题使用英文。\n")
	} else {
		value.WriteString("\n输出语言：本轮用户使用中文提问，因此 message、proposal explanation、expectedEffect 和 panel 标题使用简体中文。\n")
	}
	if request.Target != nil {
		value.WriteString("\n目标：" + request.Target.HostLabel + "（" + request.Target.Protocol + "）\n")
	}
	if conversation := strings.TrimSpace(request.SystemContext["conversation_history"]); conversation != "" {
		value.WriteString("\n用户选择继续一个本机历史会话。以下是该会话此前的上下文；请在此基础上继续回答，不要声称仍连接着旧的运行时 Agent Session：\n")
		value.WriteString(trimText(conversation, 24000) + "\n")
	}
	if len(request.SystemContext) > 0 {
		value.WriteString("环境上下文：\n")
		for key, item := range request.SystemContext {
			if key == "conversation_history" {
				continue
			}
			value.WriteString(key + "=" + trimText(item, 1000) + "\n")
		}
	}
	if len(history) > 0 {
		value.WriteString("\n此前对话：\n")
		for _, item := range history {
			value.WriteString(item.Role + ": " + trimText(item.Content, 8000) + "\n")
		}
	}
	value.WriteString("\n本轮事件：" + request.Event + "\n" + input)
	return value.String()
}

func buildCodexContinuationPrompt(request Request, input string) string {
	var value strings.Builder
	if len(request.SystemContext) > 0 {
		value.WriteString("环境上下文更新：\n")
		for key, item := range request.SystemContext {
			if key == "conversation_history" || key == "local_workspace" {
				continue
			}
			value.WriteString(key + "=" + trimText(item, 1000) + "\n")
		}
	}
	value.WriteString("\n本轮事件：" + request.Event + "\n" + input)
	return value.String()
}

func cloneContext(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func summarizeCodexResponse(response codexResponse) string {
	parts := []string{firstNonEmpty(response.Message, response.Type)}
	if response.Proposal != nil {
		parts = append(parts, "建议命令："+response.Proposal.Command)
	}
	if len(response.PanelActions) > 0 {
		parts = append(parts, fmt.Sprintf("面板操作：%d 个", len(response.PanelActions)))
	}
	return strings.Join(parts, "\n")
}

func codexRunError(runErr error, stdout, stderr []byte) error {
	return codexRunErrorLocalized(runErr, stdout, stderr, i18n.New(""))
}

func codexRunErrorLocalized(runErr error, stdout, stderr []byte, localizer i18n.Localizer) error {
	detail := string(append(append([]byte(nil), stderr...), stdout...))
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "401 unauthorized") || strings.Contains(lower, "authentication required") || strings.Contains(lower, "incorrect api key") {
		return fmt.Errorf("%s", localizer.T("本机 Codex 认证失败；请先确认 `codex exec` 可用，并检查 codex login 或 ~/.codex/config.toml"))
	}
	if strings.Contains(lower, "session not found") || strings.Contains(lower, "no session found") || strings.Contains(lower, "unknown session") || strings.Contains(lower, "rollout not found") {
		return fmt.Errorf("session not found: %s", localizer.T("本机 Codex 会话已失效"))
	}
	clean := strings.TrimSpace(detail)
	if clean == "" {
		clean = runErr.Error()
	}
	return fmt.Errorf("%s", localizer.T("本机 Codex 调用失败: %s", trimText(clean, 800)))
}

func codexSessionMissing(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "session not found")
}

func stripJSONFence(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		lines := strings.Split(value, "\n")
		if len(lines) >= 3 {
			lines = lines[1 : len(lines)-1]
			value = strings.Join(lines, "\n")
		}
	}
	return strings.TrimSpace(value)
}

func trimText(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return "…" + string(runes[len(runes)-limit:])
}

func optionalDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "：" + trimText(value, 300)
}

func newLocalID(prefix string) string {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err == nil {
		return prefix + "-" + hex.EncodeToString(data)
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
