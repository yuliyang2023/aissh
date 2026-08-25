# WRD AI SSH

`aissh` 是一个本地运行的 AI 分屏 SSH 客户端。它调用系统 OpenSSH，目标服务器不需要安装
Agent、脚本或常驻服务。

```text
┌──────────────── SSH 主终端 ────────────────┬── 监控 Tabs ───────┐
│                                            │ [系统] [进程] [磁盘]│
│ user@host:~$ 检查 nginx 状态                │                    │
│ AI ⠹ 本机 Codex 正在分析 · 2s               ├── AI 对话 ─────────┤
│ AI: nginx 正常运行                          │ 完整历史 / 特殊审查 │
└────────────────────────────────────────────┴────────────────────┘
```

## 当前 MVP

- 原样透传 OpenSSH 参数，继续使用 `~/.ssh/config`、密钥、SSH Agent 和 ProxyJump；
- 内置 PTY 与终端模拟器，不强制依赖 tmux；
- 检测到 `rz`/`sz` 的 ZMODEM 握手时自动暂停分屏重绘并切换为原始透传，完成或取消后恢复界面；
- 主终端与右上监控 Tabs、右下 AI 对话同屏；宽屏启动时 AI 对话默认展开，焦点仍留在 SSH；
- `Ctrl+N` 手动添加系统、进程、磁盘、网络、服务或自定义只读监控；
- 监控区支持自动、单屏、双栏、1×3、2×2 和自适应网格布局；大屏可自动展示 2×2、3×2 等多面板仪表盘；
- AI 可通过结构化 `panel_action` 创建和更新监控、日志 Tab；分析结论统一保留在右下 AI 对话；
- 中文开头或 `/ai` 输入自动转为 AI 请求，不发送给远端 Shell；问题、加载动画和回答会就地显示在该输入下方；
- 左侧内联提问、回答、命令建议及审批结果同步写入右下 AI 对话，点击左侧 AI 内容可查看完整上下文；
- AI 对话按 SSH 目标和会话保存在本机；右下角 `[会话]` 可选择旧会话或新建会话，并可带着旧会话上下文继续提问；
- 检测 `command not found` 后自动把失败命令和最近输出交给 AI；
- AI 命令建议紧接在左侧提问与回答下方展示，输入 `yes` 或 `no` 后回车确认；空间不足时自动改用左侧确认卡片，高风险命令在左右两侧均使用红色强调；
- 获批命令执行结束后，左侧继续显示“正在分析命令执行结果”的加载状态和最终复核结论；
- 监控优先复用 OpenSSH ControlMaster Channel，远端无需重复认证。
- AI 默认调用本机 `codex exec`，复用本机 Codex 登录与配置；目标服务器不安装任何 AI 组件。
- 可绕过 Agent，直接调用 DeepSeek 或其他 OpenAI-compatible Chat Completions 模型；aissh 自己负责会话、审批和面板动作；
- Codex 默认只读查看启动 `aissh` 时的本机当前目录，将本地源码、配置及 Git 差异与线上 SSH 输出关联分析；
- 界面支持简体中文和英文，默认根据本机区域设置选择，也可通过参数或环境变量固定语言。

## 构建

需要 Go 1.23 或更高版本：

```bash
go test ./...
go build -o bin/aissh ./cmd/aissh
```

## 使用

先确认本机 Codex 已登录并可调用：

```bash
codex exec --ephemeral "只回答 ok"
```

然后直接启动。默认 AI Provider 就是本机 Codex：

```bash
bin/aissh root@192.0.2.1
bin/aissh -p 2222 production
bin/aissh -J bastion internal-server
```

可选配置：

```bash
export AISSH_CODEX_PATH='/usr/local/bin/codex'
export AISSH_CODEX_MODEL='gpt-5.2'
bin/aissh production
```

Codex 默认将执行 `aissh` 时的本机当前目录作为只读代码工作区。因此通常先进入本地项目，再连接线上服务器：

```bash
cd /path/to/local/project
bin/aissh production

# 也可以显式指定目录，或通过 AISSH_WORKSPACE 设置
bin/aissh --workspace /path/to/local/project production
AISSH_WORKSPACE=/path/to/local/project bin/aissh production

# 不允许 Codex 查看任何本机项目文件
bin/aissh --no-workspace production
```

Codex 会以该目录作为工作根目录，并继续运行在 `read-only` 沙箱中，可以检查源码、配置、
`git status` 和 `git diff`，但不能修改本地文件。AI 生成的 `command_proposal` 始终视为线上 SSH 命令，
仍然必须在左侧审批；指定工作区不会自动上传代码、部署或同步文件。

界面语言支持 `zh-CN` 和 `en`。默认读取 `LC_ALL`、`LC_MESSAGES`、`LANG`，也可以显式指定：

```bash
bin/aissh --lang en production
AISSH_LANG=zh-CN bin/aissh production
```

语言设置会作用于界面、监控字段、审批与错误提示。AI 回答优先跟随本次提问语言：
即使界面是英文，中文提问仍会获得中文回答；难以判断时才回退到界面语言。

AI 对话历史默认保存在本机用户配置目录下的 `aissh/history`，每个 SSH 目标使用独立 JSONL 文件，
启动时最多加载最近 500 条。历史只用于右下对话框展示，不会自动发送给 Codex，也不会恢复旧的待审批命令。

```bash
# 临时关闭本次连接的历史记录
bin/aissh --no-history production

# 指定保存目录，也可以使用 AISSH_HISTORY_DIR
bin/aissh --history-dir /secure/path/aissh-history production

# 默认关闭历史记录
AISSH_HISTORY=off bin/aissh production
```

使用 Codex Provider 时，每次请求都以 Codex 临时会话运行在本机只读沙箱中，并关闭插件、浏览器、计算机操作和多 Agent。
它只返回结构化回答、命令建议与面板动作；命令仍须由用户在左侧输入 `yes` 批准后，才会写入当前 SSH PTY。

## 直连 DeepSeek 或兼容模型

直连模型模式不需要 Codex CLI，也不需要部署 Agent。aissh 调用模型的 Chat Completions API，要求模型返回
结构化 JSON，然后继续使用同一套命令审批、执行结果复核和面板安全策略。

DeepSeek：

```bash
export DEEPSEEK_API_KEY='...'
bin/aissh --agent deepseek production

# 当前默认使用 deepseek-v4-pro，也可以显式指定
bin/aissh --agent deepseek --model-name deepseek-v4-flash production
```

其他 OpenAI-compatible 模型服务：

```bash
export AISSH_MODEL_URL='https://model.example/v1'
export AISSH_MODEL_API_KEY='...'
export AISSH_MODEL_NAME='your-model'
bin/aissh --agent model production
```

`AISSH_MODEL_URL` 可以是 API Base URL，也可以直接指向 `/chat/completions`。直连模式不会把本机代码工作区
交给模型，也不会让模型获得 Shell 或文件工具；但本次提问、相关 SSH 最近输出和获批命令执行结果会发送到
所配置的模型 API。

AI 调用默认超时时间为 5 分钟，统一作用于 Codex、直连模型和 HTTP Agent：

```bash
bin/aissh --ai-timeout 10m production
AISSH_AI_TIMEOUT=600s bin/aissh production
```

也可以使用不带单位的秒数，例如 `--ai-timeout 600`。

如需继续使用 `wrd-ai-terminal` 的 Agent SSE 服务，可切换为 HTTP Provider：

```bash
export AISSH_AGENT_PROVIDER='http'
export AISSH_AGENT_URL='https://example/aiops/api/ai-console/agent/stream'
export AISSH_AGENT_TOKEN='...'
bin/aissh production
```

SSE 事件支持：

- `status`
- `thought_delta`
- `answer_delta`
- `turn`
- `panel_action`

AI 创建面板示例：

```text
event: panel_action
data: {"action":"open","panel":{"type":"monitor","title":"高 CPU 进程","collector":"process.top","intervalMs":3000}}
```

## 快捷键

| 快捷键 | 功能 |
| --- | --- |
| `Ctrl+O` | 就地展开当前命令卡片中折叠的完整命令 |
| `Ctrl+R` | 聚焦 AI 对话时打开当前 SSH 目标的历史会话列表 |
| `Ctrl+T` | 切换右上监控 Tab |
| `Ctrl+L` | 聚焦监控区时切换自动、单屏、双栏、1×3、2×2 和网格布局 |
| `Ctrl+N` | 新增监控 Tab |
| `Esc` | 返回 SSH 主终端 |
| `yes` + Enter | 批准并执行左侧 AI 命令建议 |
| `no` + Enter | 拒绝左侧 AI 命令建议 |

鼠标操作：

- 点击左侧 SSH、右上监控区或右下 AI 区可直接切换焦点；点击左侧内联 AI 内容会展开并聚焦右下对话；
- 点击右下 AI 标题栏的 `[会话]` 浏览历史，点击旧会话即可加载并继续提问；
- 点击监控 Tab 切换面板，点击 `+` 新增面板；
- 点击右上角布局标签切换布局；多面板布局下点击面板可将其设为当前 Tab；
- 双栏、1×3、2×2 和网格布局中，每个卡片标题栏都有 `[换]`，可单独指定该块显示哪个监控面板；
- 点击 `[ yes 执行 ]` 或 `[ no 拒绝 ]` 审批 AI 命令建议；
- 在左侧 SSH 区滚轮浏览本机保存的终端历史，在监控标题栏滚轮切换 Tab，在监控卡片内滚动内容，
  在 AI 区滚轮浏览对话历史；
- SSH、AI 和监控卡片内容超过窗口高度时会显示右缘滚动条；SSH/AI 可点击轨道跳转；
- 启用鼠标报告后，多数终端可按住 `Shift` 再拖动来选择文本。

命令行发起 AI 请求时，等待期间会在原输入下一行显示旋转状态和已用时间。完成后回答会作为左侧
终端的内联记录保留，后续 SSH 提示符和命令从回答下方继续，不会因为再次键入而清空；点击左侧 AI
内容可打开右下角查看完整对话。左侧内联区域按当前内容动态计算高度：加载状态只占一行，流式回答和
命令卡片根据实际换行逐步扩展；位于终端底部时视口会随内容增长，较长内容在左侧折叠，完整内容仍
保留在右下对话框。

AI 命令建议使用独立命令卡片展示：管道、`&&` 和长参数优先在语义边界折行，续行带有醒目标记；
左侧空间不足时会明确显示剩余折叠行数；按 `Ctrl+O` 或点击 `[展开全部]` 会在当前卡片中就地展开，
不会切换到右下 AI 对话。折行只影响展示，
批准执行时始终使用 Agent 返回的原始命令。

SSH 输出采用最高约 30 FPS 的合帧刷新，并按行覆盖左侧终端，不会因远端输出被拆成多个小数据块而
反复清空整个 SSH 区域；支持同步更新的终端还会以完整帧呈现，减少闪烁和撕裂。
左侧 SSH 历史保存在本机内存中，默认最多保留 20000 行；查看历史期间出现新输出时，当前阅读位置会
保持不动，继续键盘输入则自动返回实时终端。

```bash
# 临时调整本次连接的回滚行数
bin/aissh --scrollback-lines 50000 production

# 或设置默认值
AISSH_SCROLLBACK_LINES=50000 bin/aissh production
```

终端宽度小于 92 列时，`aissh` 自动切换为紧凑模式：SSH 和右侧工作区使用全屏切换，避免把
远端 PTY 压缩得不可用。

## 安全边界

- AI 创建的监控只允许执行保守判定为只读的命令；
- 本机 Codex 始终运行在只读沙箱中；未配置本地项目时使用临时工作区，并且不直接连接或操作目标服务器；
- 本机工作区只允许 Codex 只读检查；本地修改、上传和部署均不会自动执行，可用 `--no-workspace` 完全关闭；
- 右下 AI 对话在宽屏模式下默认展开且不会自动收起；普通回答与命令建议仍优先就地显示在左侧；
- AI不能关闭用户创建或固定的面板；
- 用户创建的面板默认不把数据发送给 AI；
- AI 建议不会自动执行；
- 监控命令具有超时、最小刷新间隔和输出限制；
- Codex 登录信息和 HTTP Agent Token 都只留在本机，不发送到目标服务器。
- 直连模型的 API Key 只用于本机发起 Bearer 请求；AI 提问和相关 SSH 输出会发送给所配置的模型服务；
- AI 历史文件只写本机，目录权限为 `0700`、文件权限为 `0600`；内容可能包含提问、回答和命令建议，敏感环境可使用 `--no-history`。

## 暂未包含

- 持续 `tail -F` 类型的独立流式 Channel；当前日志面板使用周期快照；
- 第二个交互式 Shell Tab；
- 面板布局拖拽和模板持久化；
- Markdown/PDF 报告导出；
- Telnet 和网络设备专用输入模式。
