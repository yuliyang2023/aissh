package i18n

import (
	"fmt"
	"os"
	"strings"
	"unicode"
)

const (
	Chinese = "zh-CN"
	English = "en"
)

type Localizer struct {
	language string
}

func New(language string) Localizer {
	return Localizer{language: Normalize(language)}
}

func Normalize(language string) string {
	value := strings.ToLower(strings.TrimSpace(language))
	value = strings.ReplaceAll(value, "_", "-")
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		value = value[:dot]
	}
	if at := strings.IndexByte(value, '@'); at >= 0 {
		value = value[:at]
	}
	switch {
	case strings.HasPrefix(value, "en"):
		return English
	case strings.HasPrefix(value, "zh"):
		return Chinese
	default:
		// Keep the original Chinese UI as the compatibility fallback.
		return Chinese
	}
}

func DetectEnvironment() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "en") || strings.HasPrefix(lower, "zh") {
			return Normalize(value)
		}
	}
	return Chinese
}

func Resolve(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "auto") || strings.TrimSpace(value) == "" {
		return DetectEnvironment()
	}
	return Normalize(value)
}

// PreferredResponseLanguage follows the language used in the current user
// message. Mixed Chinese/English operational questions are treated as Chinese;
// text without natural-language letters falls back to the configured UI locale.
func PreferredResponseLanguage(text, fallback string) string {
	hasLatin := false
	for _, current := range text {
		if unicode.Is(unicode.Han, current) {
			return Chinese
		}
		if unicode.Is(unicode.Latin, current) && unicode.IsLetter(current) {
			hasLatin = true
		}
	}
	if hasLatin {
		return English
	}
	return Resolve(fallback)
}

func (l Localizer) Language() string { return l.language }
func (l Localizer) IsEnglish() bool  { return l.language == English }

// T uses the Simplified Chinese message as a stable gettext-style message ID.
// Missing translations deliberately fall back to that message.
func (l Localizer) T(message string, args ...any) string {
	template := message
	if l.IsEnglish() {
		if translated, ok := englishCatalog[message]; ok {
			template = translated
		}
	}
	if len(args) == 0 {
		return template
	}
	return fmt.Sprintf(template, args...)
}

var englishCatalog = map[string]string{
	"SSH 写入失败（%d/%d 字节）：%s":                   "SSH write failed (%d/%d bytes): %s",
	"命令结果尚未确认；等待新的 Shell 提示符，可按 Ctrl+C 重新同步。": "Command result is unconfirmed; waiting for a fresh shell prompt. Press Ctrl+C to resynchronize.",
	"仍未收到命令完成标记；按 Ctrl+C 中断并重新同步 Shell。":      "Still waiting for command completion. Press Ctrl+C to interrupt and resynchronize the shell.",
	"等待新的 Shell 提示符；Ctrl+C 重新同步":              "Waiting for a fresh shell prompt; Ctrl+C Resynchronize",
	"正在等待命令结果；Ctrl+C 中断并恢复":                   "Waiting for command result; Ctrl+C Interrupt and recover",
	"系统":       "System",
	"进程":       "Processes",
	"磁盘":       "Disks",
	"网络":       "Network",
	"服务":       "Services",
	"自定义":      "Custom",
	"单屏":       "Single",
	"双栏":       "Split",
	"1×3":      "1×3",
	"2×2":      "2×2",
	"网格":       "Grid",
	"自动":       "Auto",
	"监控 ":      "Monitor ",
	" [布局:%s]": " [Layout:%s]",
	"[换]":      "[Change]",
	"选择第 %d 块显示内容 · %s布局": "Choose content for tile %d · %s layout",
	"Esc 取消":          "Esc Cancel",
	"没有监控面板":          "No monitor panels",
	"调查报告将在这里生成。":     "Report output appears here.",
	"✦ AI 助手":         "✦ AI Assistant",
	"[会话]":            "[Sessions]",
	"会话历史":            "Session history",
	"+ 新建会话":          "+ New session",
	"未知时间":            "Unknown time",
	" · 当前":           " · Current",
	"%s  %s · %d 条%s": "%s  %s · %d messages%s",
	"↑/↓ 选择 · Enter 打开 · N 新建 · Esc 返回": "↑/↓ Select · Enter Open · N New · Esc Back",
	"○ 待命":   "○ Ready",
	"● 正在处理": "● Working",
	"● 已同步":  "● Synced",
	"你":      "You",
	"! 错误":   "! Error",
	"◆ 系统":   "◆ System",
	"  ! 高风险命令正在左侧等待确认": "  ! A high-risk command awaits confirmation on the left",
	"╭─ ⚑ 命令审查 · %s ":   "╭─ ⚑ Command review · %s ",
	"AI 已生成命令建议":        "AI generated a command proposal",
	"│ 预期  ":            "│ Expected  ",
	"╭─ 输入":             "╭─ Input",
	" · AI 正在处理":        " · AI is working",
	"滚动模式 · Ctrl+] 恢复选择": "Mouse mode · Ctrl+] Restore selection",
	"选择模式 · Ctrl+] 开启滚动": "Selection mode · Ctrl+] Enable scrolling",
	"› AI 对话已打开":        "› AI conversation is open",
	"› AI 完整对话":         "› Full AI conversation",
	"AI 已就绪（%s）。中文开头或 /ai 会自动提问。": "AI is ready (%s). Use Chinese input or /ai to ask from the SSH prompt.",
	"SSH 已结束：%s":      "SSH ended: %s",
	"SSH 连接失败或异常结束（%s）": "SSH connection failed or ended unexpectedly (%s)",
	"不可用":             " unavailable",
	"[Ctrl+Shift+- 收起右栏]": "[Ctrl+Shift+- Hide sidebar]",
	"[Ctrl+Shift+- 展开右栏]": "[Ctrl+Shift+- Show sidebar]",
	"实时":              "Live",
	"等待连接":            "Waiting",
	"采集错误:":           "Collection error:",
	"采集失败":            "Collection failed",
	"主 SSH 建立后会自动重试。": "Retries automatically after the main SSH connection is established.",
	"等待监控数据……":        "Waiting for monitor data…",
	"AI创建":            "Created by AI",
	"用户创建":            "Created by user",
	"内置":              "Built-in",
	"新增面板":            "Add panel",
	"1  系统概览":         "1  System overview",
	"2  进程排行":         "2  Top processes",
	"3  磁盘使用":         "3  Disk usage",
	"4  网络状态":         "4  Network status",
	"5  服务状态":         "5  Service status",
	"6  自定义只读命令":      "6  Custom read-only command",
	"新增自定义监控":         "Add custom monitor",
	"监控命令必须是只读命令。":    "Monitor commands must be read-only.",
	"输入只读监控命令":        "Enter a read-only monitor command",
	"命令不是只读命令，请修改后重试": "The command is not read-only; edit it and try again",
	"只读":                     "Read-only",
	"低风险":                    "Low risk",
	"中风险":                    "Medium risk",
	"高风险":                    "High risk",
	"风险未知":                   "Unknown risk",
	"本机 %s 正在分析":             "Local %s is analyzing",
	"处理完成。":                  "Completed.",
	"AI 错误: ":                "AI error: ",
	"… 完整内容见 AI 对话":          "… Full content is in the AI conversation",
	"AI 建议命令  [%s]  %d 字符":   "AI command proposal  [%s]  %d chars",
	"说明  ":                   "Details  ",
	"命令建议已结束":                "Command proposal closed",
	"AI 命令确认    %s    %d 字符": "AI command confirmation    %s    %d chars",
	"AI 建议执行以下命令":            "AI suggests running this command",
	"预期  ":                   "Expected  ",
	"输入 yes/no > ":           "Enter yes/no > ",
	"[ yes 执行 ]  [ no 拒绝 ]":  "[ yes Run ]  [ no Reject ]",
	"[ yes 执行 ]":             "[ yes Run ]",
	"[ no 拒绝 ]":              "[ no Reject ]",
	"[展开全部]":                 "[Expand all]",
	"$ … [展开全部] · Ctrl+O":    "$ … [Expand all] · Ctrl+O",
	"  … [展开全部] · Ctrl+O · 余%d行":                                        "  … [Expand all] · Ctrl+O · %d more lines",
	"$ … 命令超出当前屏幕":                                                      "$ … Command exceeds the current screen",
	"  … 命令超出当前屏幕 · 余%d行":                                               "  … Command exceeds the current screen · %d more lines",
	"请输入 yes 或 no 后回车":                                                  "Enter yes or no, then press Enter",
	"请选择 yes（执行）或 no（拒绝）":                                               "Choose yes (run) or no (reject)",
	"SSH 历史  ↑ %d / %d 行  键盘输入返回实时终端":                                   "SSH history  ↑ %d / %d lines  Type to return to the live terminal",
	"AI %s 本机 %s 正在处理":                                                  "AI %s Local %s is working",
	"AI 回答已显示在原输入下方；继续输入即可返回 SSH":                                       "The AI answer is shown below the original input; keep typing to return to SSH",
	"命令确认  yes 执行 · no 拒绝 · Enter 确认":                                   "Confirm command  yes Run · no Reject · Enter Confirm",
	"AI  Enter 发送 · Shift+Enter/Ctrl+J 换行 · Ctrl+R 历史 · Esc 返回 SSH": "AI  Enter Send · Shift+Enter/Ctrl+J New line · Ctrl+R History · Esc SSH",
	"历史会话  ↑/↓ 选择 · Enter 继续 · N 新建 · Esc 返回":                         "Sessions  ↑/↓ Select · Enter Continue · N New · Esc Back",
	"监控  Ctrl+T 切换 · Ctrl+L 布局 · Ctrl+N 新增 · Esc 返回 SSH":             "Monitor  Ctrl+T Switch · Ctrl+L Layout · Ctrl+N Add · Esc SSH",
	"选择分屏  1–9 选择 · Esc 取消":                                            "Choose tile  1–9 Select · Esc Cancel",
	"新增监控  1–6 选择 · Esc 取消":                                            "Add monitor  1–6 Select · Esc Cancel",
	"%s  Enter 创建 · Ctrl+U 清空 · Esc 取消":                                 "%s  Enter Create · Ctrl+U Clear · Esc Cancel",
	"SSH  /ai AI 对话 · Ctrl+T 切换监控 · Ctrl+N 新增监控":                       "SSH  /ai AI chat · Ctrl+T Switch monitor · Ctrl+N Add monitor",
	"正在连接本机 %s":                                                         "Connecting to local %s",
	"AI 正在处理上一个请求，请稍后重试。":                                               "AI is processing the previous request; try again shortly.",
	"AI 正在处理上一个请求。":                                                     "AI is processing the previous request.",
	"正在连接 AI":                                                           "Connecting to AI",
	"当前不在安全的 Shell 提示符；可输入 no 拒绝，或稍后再确认":                                "The shell is not at a safe prompt; enter no to reject or confirm later",
	"当前不在安全的 Shell 提示符，暂时不能执行；请输入 no 拒绝":                                "The shell is not at a safe prompt, so execution is unavailable; enter no to reject",
	"正在批准命令":                 "Approving command",
	"正在确认并准备写入 SSH……":        "Confirming and preparing to send to SSH…",
	"批准失败：%s":                "Approval failed: %s",
	"已批准执行命令：%s":             "Approved command: %s",
	"已批准执行：%s":               "Approved: %s",
	"正在拒绝命令":                 "Rejecting command",
	"正在拒绝命令建议……":             "Rejecting command proposal…",
	"已拒绝命令建议：%s":             "Rejected command proposal: %s",
	"拒绝失败：%s":                "Rejection failed: %s",
	"已生成命令建议，请在左侧 SSH 窗口确认。": "A command proposal was generated; confirm it in the left SSH pane.",
	"命令：%s":                  "Command: %s",
	"风险：%s":                  "Risk: %s",
	"AI 正在分析执行结果":            "AI is analyzing the execution result",
	"AI 复核命令：%s":             "AI review command: %s",
	"正在分析命令执行结果":             "Analyzing command output",
	"分析命令结果":                 "Analyze command output",
	"AI 请求的监控命令被只读策略拒绝。":     "The AI-requested monitor command was rejected by the read-only policy.",
	"AI 面板":                  "AI Panel",
	"AI 报告":                  "AI Report",
	"AI 更新":                  "AI Update",
	"AI 已完成":             "AI finished",
	"回答已准备好，请返回 aissh 查看。":    "Your answer is ready. Return to aissh to view it.",
	"AI 等待命令确认":               "AI awaiting command approval",
	"命令建议已准备好，请返回 aissh 确认。":  "A command is ready for review. Return to aissh to approve or reject it.",
	"AI 等待回复":                 "AI awaiting your reply",
	"AI 需要补充信息，请返回 aissh 回复。": "AI needs more information. Return to aissh to reply.",
	"AI 处理失败":                 "AI request failed",
	"请返回 aissh 查看错误信息。":       "Return to aissh to view the error.",
	"刚才命令 %q 执行失败：%s。请分析原因并给出安全的下一步。": "The previous command %q failed: %s. Analyze the cause and suggest a safe next step.",
	"监控命令被只读策略拒绝":                     "Monitor command rejected by the read-only policy",
	"主会话监控已暂停":                        "Monitoring through the primary session is paused",
	"监控查询超时，主会话监控已暂停":                 "Monitor query timed out; monitoring through the primary session is paused",
	"监控已暂停，等待 Shell 提示符；可按 Ctrl+C 中断": "Monitoring paused; waiting for a shell prompt. Press Ctrl+C to interrupt",
	"主机":                              "Host",
	"运行":                              "Uptime",
	"负载":                              "Load",
	"内存":                              "Memory",
	"根分区":                             "Root disk",
	"进程数":                             "Processes",
	"找不到本机 Codex CLI；请先安装 Codex 或设置 AISSH_CODEX_PATH": "Local Codex CLI was not found; install Codex or set AISSH_CODEX_PATH",
	"正在调用本机 Codex":      "Calling local Codex",
	"本机 Codex 返回了空命令建议": "Local Codex returned an empty command proposal",
	"已拒绝该命令建议。你可以继续描述希望采用的处理方式。":                                                "The command proposal was rejected. You can describe another approach.",
	"本机 Codex 调用超时（%s）":                                                         "Local Codex timed out (%s)",
	"本机 Codex 没有返回结果%s":                                                         "Local Codex returned no result%s",
	"无法解析本机 Codex 的结构化结果: %v":                                                   "Could not parse the structured response from local Codex: %v",
	"本机 Codex 认证失败；请先确认 `codex exec` 可用，并检查 codex login 或 ~/.codex/config.toml": "Local Codex authentication failed; verify `codex exec`, codex login, and ~/.codex/config.toml",
	"本机 Codex 调用失败: %s":                                                         "Local Codex failed: %s",
	"本机 Codex 会话已关闭":                                                            "The local Codex session is closed",
	"本机 Codex 会话已失效":                                                            "The local Codex session is no longer available",
	"本机 Codex 没有返回会话 ID":                                                        "Local Codex returned no session ID",
	"本机 Codex 恢复了意外的会话：%s":                                                      "Local Codex resumed an unexpected session: %s",
	"找不到本机 Claude CLI；请先安装 Claude Code 或设置 AISSH_CLAUDE_PATH":                   "Local Claude CLI was not found; install Claude Code or set AISSH_CLAUDE_PATH",
	"正在调用本机 Claude":                                                             "Calling local Claude",
	"本机 Claude 返回了空命令建议":                                                        "Local Claude returned an empty command proposal",
	"本机 Claude 调用超时（%s）":                                                        "Local Claude timed out (%s)",
	"本机 Claude 没有返回结果%s":                                                        "Local Claude returned no result%s",
	"无法解析本机 Claude 的结构化结果: %v":                                                  "Could not parse the structured response from local Claude: %v",
	"本机 Claude 认证失败；请先确认 `claude -p` 可用并已登录，或设置 ANTHROPIC_API_KEY":              "Local Claude authentication failed; verify `claude -p` works and you are logged in, or set ANTHROPIC_API_KEY",
	"本机 Claude 调用失败: %s":                                                        "Local Claude failed: %s",
	"本机 Claude 会话已关闭":                                                           "The local Claude session is closed",
	"本机 Claude 会话已失效":                                                           "The local Claude session is no longer available",
	"本机 Claude 没有返回会话 ID":                                                       "Local Claude returned no session ID",
	"本机 AI 历史读取失败：%s":                                                           "Could not read local AI history: %s",
	"本机 AI 历史写入失败：%s":                                                           "Could not write local AI history: %s",
	"发现 %d 个本机 AI 历史会话 · 在 AI 对话中按 Ctrl+R 查看":                                   "Found %d local AI sessions · press Ctrl+R in the AI conversation to browse",
	"已切换到历史会话：%s；后续提问会延续该会话上下文。":                                                "Switched to session: %s; future questions will continue with its context.",
	"已新建 AI 会话。":                                                                "Started a new AI session.",
	"无法访问本机代码工作区：%s":                                                            "Could not access the local code workspace: %s",
	"本机代码工作区（只读）：%s":                                                            "Local code workspace (read-only): %s",
	"本机代码工作区（可修改）：%s":                                                           "Local code workspace (writable): %s",
	"当前本机代码工作区（只读）：%s":                                                          "Current local code workspace (read-only): %s",
	"当前本机代码工作区（可修改）：%s":                                                         "Current local code workspace (writable): %s",
	"当前未启用本机代码工作区；使用 /workspace 本机路径 开启":                                        "No local code workspace is enabled; use /workspace LOCAL_PATH to enable it",
	"本机代码工作区已切换（只读）：%s；后续 %s 请求将使用此目录":                                          "Local code workspace changed (read-only): %s; subsequent %s requests will use it",
	"本机代码工作区已切换（可修改）：%s；后续 %s 请求将使用此目录":                                         "Local code workspace changed (writable): %s; subsequent %s requests will use it",
	"已关闭本机代码工作区；后续 %s 请求不会读取本机项目":                                               "Local code workspace disabled; subsequent %s requests will not read local projects",
	"当前 AI 后端不支持本机代码工作区":                                                        "The current AI backend does not support a local code workspace",
	"AI 正在处理请求；请在本次请求结束后切换工作区":                                                  "AI is processing a request; change the workspace after it finishes",
	"直连模型模式：AI 提问和相关 SSH 输出会发送到 %s；本机代码工作区不会发送。":                                "Direct model mode: AI prompts and relevant SSH output are sent to %s; the local code workspace is not sent.",
	"直连模型未配置；请设置模型地址和模型名称":                                                      "The direct model is not configured; set its API URL and model name",
	"直连模型缺少 API Key":                                                            "The direct model API key is missing",
	"正在调用直连模型 %s":                                                               "Calling direct model %s",
	"直连模型返回了空命令建议":                                                              "The direct model returned an empty command proposal",
	"直连模型调用失败：%s":                                                               "Direct model call failed: %s",
	"无法解析直连模型响应：%s":                                                             "Could not parse the direct model response: %s",
	"直连模型返回错误：%s":                                                               "Direct model returned an error: %s",
	"直连模型没有返回内容":                                                                "The direct model returned no content",
	"无法解析直连模型的结构化结果：%s":                                                         "Could not parse the direct model structured result: %s",
}

func init() {
	englishCatalog["always-yes 已开启：非高风险命令经 %d 次独立复核后自动执行；高风险、未知或复核失败仍需手动确认。"] = "always-yes is enabled: non-high-risk commands run automatically after %d independent reviews; high-risk, unknown, or failed reviews still require manual confirmation."
	englishCatalog["风险等级未知，always-yes 未自动批准；请手动确认"] = "The risk level is unknown; always-yes did not approve it. Confirm manually"
	englishCatalog["高风险命令不会由 always-yes 自动批准；请手动确认"] = "High-risk commands are never approved by always-yes. Confirm manually"
	englishCatalog["当前 AI 后端不支持独立风险复核，always-yes 未自动批准；请手动确认"] = "This AI backend does not support independent risk reviews; always-yes did not approve it. Confirm manually"
	englishCatalog["always-yes 正在进行风险复核（%d/%d）"] = "always-yes risk review (%d/%d)"
	englishCatalog["always-yes 风险复核失败，未自动批准；请手动确认：%s"] = "always-yes risk review failed; the command was not approved. Confirm manually: %s"
	englishCatalog["always-yes 风险复核结果不完整，未自动批准；请手动确认"] = "always-yes risk reviews were incomplete; the command was not approved. Confirm manually"
	englishCatalog["多次风险复核判定为高风险，always-yes 未自动批准；请手动确认"] = "Repeated risk reviews classified this command as high risk; always-yes did not approve it. Confirm manually"
	englishCatalog["always-yes 已通过 %d 次独立风险复核，正在自动批准命令。"] = "always-yes passed %d independent risk reviews and is automatically approving the command."
}
