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
	Proposals        map[string]Proposal
	ResponseLanguage string
}

type codexRunner func(context.Context, string, []string, string) ([]byte, []byte, error)

// CodexProvider invokes the locally installed Codex CLI. The Codex process
// runs in an ephemeral, read-only workspace and only returns structured advice;
// aissh remains solely responsible for executing approved commands over SSH.
type CodexProvider struct {
	Path               string
	Model              string
	Timeout            time.Duration
	Language           string
	Workspace          string
	OutputContextChars int

	mu       sync.Mutex
	sessions map[string]*codexSession
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
	p.Workspace = strings.TrimSpace(value)
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

	if callbacks.OnStatus != nil {
		callbacks.OnStatus(p.localizer().T("正在调用本机 Codex"))
	}
	prompt := buildCodexPrompt(request, history, input)
	result, err := p.invoke(ctx, prompt, workspace)
	if err != nil {
		if created {
			p.mu.Lock()
			delete(p.sessions, session.ID)
			p.mu.Unlock()
		}
		return Turn{}, err
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

func (p *CodexProvider) Action(_ context.Context, request Request) (Turn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
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
		session.History = append(session.History, codexHistory{Role: "user", Content: "用户批准执行命令：" + proposal.Command})
		return Turn{Type: "approved", Session: Session{ID: session.ID, Status: "running"}}, nil
	case "reject":
		delete(session.Proposals, request.ProposalID)
		session.History = append(session.History, codexHistory{Role: "user", Content: "用户拒绝执行命令：" + proposal.Command})
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
		value := &codexSession{ID: newLocalID("codex"), Proposals: make(map[string]Proposal)}
		p.sessions[value.ID] = value
		return value, true, nil
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
			delete(session.Proposals, request.ProposalID)
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

func (p *CodexProvider) invoke(parent context.Context, prompt, workspace string) (codexResponse, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	schema, err := os.CreateTemp("", "aissh-codex-schema-*.json")
	if err != nil {
		return codexResponse{}, err
	}
	schemaPath := schema.Name()
	defer os.Remove(schemaPath)
	if _, err := schema.WriteString(codexResponseSchema); err != nil {
		_ = schema.Close()
		return codexResponse{}, err
	}
	if err := schema.Close(); err != nil {
		return codexResponse{}, err
	}

	args := []string{
		"exec", "--json", "--ephemeral", "--sandbox", "read-only",
		"--skip-git-repo-check", "--ignore-rules",
		"--disable", "plugins", "--disable", "remote_plugin", "--disable", "apps",
		"--disable", "browser_use", "--disable", "computer_use", "--disable", "image_generation",
		"--disable", "multi_agent", "--disable", "hooks",
		"-C", codexWorkingDirectory(workspace), "--output-schema", schemaPath,
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	args = append(args, "-")
	stdout, stderr, runErr := p.run(ctx, p.Path, args, prompt)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return codexResponse{}, fmt.Errorf("%s", p.localizer().T("本机 Codex 调用超时（%s）", timeout))
	}
	response, parseErr := parseCodexJSONLLocalized(stdout, p.localizer())
	if runErr != nil {
		return codexResponse{}, codexRunErrorLocalized(runErr, stdout, stderr, p.localizer())
	}
	if parseErr != nil {
		return codexResponse{}, parseErr
	}
	return response, nil
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
	return parseCodexJSONLLocalized(data, i18n.New(""))
}

func parseCodexJSONLLocalized(data []byte, localizer i18n.Localizer) (codexResponse, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var assistantText string
	var lastError string
	for scanner.Scan() {
		var event struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Item    struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Message string `json:"message"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
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
		return codexResponse{}, err
	}
	if strings.TrimSpace(assistantText) == "" {
		return codexResponse{}, fmt.Errorf("%s", localizer.T("本机 Codex 没有返回结果%s", optionalDetail(lastError)))
	}
	assistantText = stripJSONFence(assistantText)
	var response codexResponse
	if err := json.Unmarshal([]byte(assistantText), &response); err != nil {
		return codexResponse{}, fmt.Errorf("%s", localizer.T("无法解析本机 Codex 的结构化结果: %v", err))
	}
	return response, nil
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
2. 命令必须适合在目标服务器的当前交互式 Shell 中执行，不得包含交互式全屏程序。
3. 有破坏性、重启、删除、覆盖、权限或网络策略变更风险时，risk 必须如实标为 high 或 critical，并在 explanation 中说明。
4. 可用 panelActions 创建 monitor 或 log Tab，command 必须严格只读；分析结论和报告内容统一写入 message，不创建报告面板；不需要监控面板时返回空数组。
5. 如果信息不足，返回 ask_user 并在 message 中提出一个简短问题；能直接回答时返回 finished。
6. 只返回符合 JSON Schema 的对象，不要 Markdown 代码围栏。

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
	clean := strings.TrimSpace(detail)
	if clean == "" {
		clean = runErr.Error()
	}
	return fmt.Errorf("%s", localizer.T("本机 Codex 调用失败: %s", trimText(clean, 800)))
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
