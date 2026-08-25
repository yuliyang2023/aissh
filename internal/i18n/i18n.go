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
	"› AI 对话已打开":        "› AI conversation is open",
	"› 点击打开完整对话":        "› Click to open full conversation",
	"AI 已就绪（%s）。点击右下对话；中文开头或 /ai 会自动提问。": "AI is ready (%s). Click the bottom-right conversation; use /ai to ask from the SSH prompt.",
	"SSH 已结束：%s":      "SSH ended: %s",
	"不可用":             " unavailable",
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
	"只读":                         "Read-only",
	"低风险":                        "Low risk",
	"中风险":                        "Medium risk",
	"高风险":                        "High risk",
	"风险未知":                       "Unknown risk",
	"本机 Codex 正在分析":              "Local Codex is analyzing",
	"处理完成。":                      "Completed.",
	"AI 错误: ":                    "AI error: ",
	"… 点击查看完整对话":                 "… Click to view full conversation",
	"AI 建议命令  [%s]  %d 字符":       "AI command proposal  [%s]  %d chars",
	"说明  ":                       "Details  ",
	"命令建议已结束":                    "Command proposal closed",
	"AI 命令确认    %s    %d 字符":     "AI command confirmation    %s    %d chars",
	"AI 建议执行以下命令":                "AI suggests running this command",
	"预期  ":                       "Expected  ",
	"输入 yes/no > ":               "Enter yes/no > ",
	"[ yes 执行 ]  [ no 拒绝 ]":      "[ yes Run ]  [ no Reject ]",
	"[ yes 执行 ]":                 "[ yes Run ]",
	"[ no 拒绝 ]":                  "[ no Reject ]",
	"[展开全部]":                     "[Expand all]",
	"$ … [展开全部] · Ctrl+O":        "$ … [Expand all] · Ctrl+O",
	"  … [展开全部] · Ctrl+O · 余%d行": "  … [Expand all] · Ctrl+O · %d more lines",
	"$ … 命令超出当前屏幕":               "$ … Command exceeds the current screen",
	"  … 命令超出当前屏幕 · 余%d行":        "  … Command exceeds the current screen · %d more lines",
	"请输入 yes 或 no 后回车":           "Enter yes or no, then press Enter",
	"请选择 yes（执行）或 no（拒绝）":        "Choose yes (run) or no (reject)",
	"SSH 历史  ↑ %d / %d 行  滚轮向下回到底部；键盘输入也会返回实时终端":                  "SSH history  ↑ %d / %d lines  Scroll down or type to return to the live terminal",
	"AI %s 本机 Codex 正在处理；点击左侧 AI 内容可查看完整对话":                       "AI %s Local Codex is working; click the inline AI content to view the full conversation",
	"AI 回答已显示在原输入下方；继续输入即可返回 SSH，点击内容查看完整对话":                      "The AI answer is shown below the original input; keep typing to return to SSH, or click it for the full conversation",
	"AI 命令确认：输入 yes 执行，输入 no 拒绝，然后按 Enter":                        "AI command confirmation: enter yes to run or no to reject, then press Enter",
	"AI 对话  Enter发送  Ctrl+R历史会话  Esc返回SSH  Ctrl+T查看监控":            "AI conversation  Enter Send  Ctrl+R Sessions  Esc SSH  Ctrl+T Monitors",
	"AI 历史会话  ↑/↓选择  Enter继续  N新建  Esc返回对话":                       "AI sessions  ↑/↓ Select  Enter Continue  N New  Esc Conversation",
	"监控面板  Ctrl+T切换Tab  Ctrl+L切换布局  点击右下AI对话  Ctrl+N新增  Esc返回SSH": "Monitor panels  Ctrl+T Next tab  Ctrl+L Layout  Click bottom-right AI  Ctrl+N Add  Esc SSH",
	"选择该分屏显示的监控：按数字或鼠标点击；Esc 取消":                                  "Choose the monitor for this tile: press a number or click; Esc cancels",
	"新增: 1系统 2进程 3磁盘 4网络 5服务 6自定义  Esc取消":                         "Add: 1 System 2 Processes 3 Disks 4 Network 5 Services 6 Custom  Esc Cancel",
	"%s  Enter创建  Esc取消": "%s  Enter Create  Esc Cancel",
	"鼠标可点击AI对话  Ctrl+T监控Tab  Ctrl+N新增监控  中文或 /ai 自动提问": "Click AI conversation  Ctrl+T Monitor tab  Ctrl+N Add monitor  Use /ai to ask",
	"正在连接本机 Codex":        "Connecting to local Codex",
	"AI 正在处理上一个请求，请稍后重试。": "AI is processing the previous request; try again shortly.",
	"AI 正在处理上一个请求。":       "AI is processing the previous request.",
	"正在连接 AI":             "Connecting to AI",
	"当前不在安全的 Shell 提示符；可输入 no 拒绝，或稍后再确认": "The shell is not at a safe prompt; enter no to reject or confirm later",
	"当前不在安全的 Shell 提示符，暂时不能执行；请输入 no 拒绝": "The shell is not at a safe prompt, so execution is unavailable; enter no to reject",
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
	"刚才命令 %q 执行失败：%s。请分析原因并给出安全的下一步。": "The previous command %q failed: %s. Analyze the cause and suggest a safe next step.",
	"监控命令被只读策略拒绝":                     "Monitor command rejected by the read-only policy",
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
	"本机 AI 历史读取失败：%s":                                                           "Could not read local AI history: %s",
	"本机 AI 历史写入失败：%s":                                                           "Could not write local AI history: %s",
	"发现 %d 个本机 AI 历史会话 · 点击 [会话] 查看":                                            "Found %d local AI sessions · click [Sessions] to browse",
	"已切换到历史会话：%s；后续提问会延续该会话上下文。":                                                "Switched to session: %s; future questions will continue with its context.",
	"已新建 AI 会话。":                                                                "Started a new AI session.",
	"无法访问本机代码工作区：%s":                                                            "Could not access the local code workspace: %s",
	"本机代码工作区（只读）：%s":                                                            "Local code workspace (read-only): %s",
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
