# WRD AI SSH

[English](README.md) | **简体中文**

> 把 AI 助手、实时监控和安全审批，带进你已经熟悉的 SSH。

`aissh` 是一个运行在本机的 AI SSH 工作台。它保留原生 SSH 的连接方式，同时把故障分析、命令建议、
实时监控和 AI 对话整合到同一个终端界面中。

目标服务器不需要安装 Agent、脚本或常驻服务。像平时一样连接，然后直接开始工作。

```text
┌────────────────── SSH 主终端 ──────────────────┬──── 实时监控 ────┐
│                                                │ [系统] [进程] [磁盘]│
│ root@server:~# 检查 nginx 为什么响应变慢         │                   │
│ AI ⠹ 正在结合服务器状态与本地代码分析……           ├──── AI 对话 ─────┤
│ AI: 发现上游连接超时，建议先检查错误日志与连接数。  │ 完整对话与审查记录 │
│                                                │                   │
└────────────────────────────────────────────────┴───────────────────┘
```

## 一条命令，升级你的 SSH

你不需要改变原来的 SSH 使用习惯：

```bash
aissh root@192.0.2.1
aissh -p 2222 production
aissh -J bastion internal-server
```

`~/.ssh/config`、密钥、SSH Agent、跳板机等原有配置都可以继续使用。

## 核心体验

### ✦ AI 就在命令旁边

直接在 SSH 提示符输入中文问题，回答会显示在原输入下方，不会把你带离当前操作位置。

```text
root@server:~# 帮我看看 nginx 最近有没有异常
AI: nginx 正常运行，但最近 10 分钟出现了多次上游超时……
```

英文问题可以使用 `/ai`：

```text
/ai check why this service keeps restarting
```

左侧适合快速处理当前问题，右下角保留完整对话、历史上下文和审查记录。两边内容自动同步。

### ⚡ 出错时主动帮你分析

遇到 `command not found` 等常见错误时，`aissh` 可以自动结合失败命令和最近输出发起分析，
不用再手动复制日志、切换窗口、重新描述现场。

执行 AI 建议的命令后，它还会继续分析执行结果，帮助你判断问题是否真正解决。

### 🛡️ AI 建议，用户决策

AI 生成的命令不会自动执行，而是先在左侧终端中展示用途、预期效果和风险等级。

- 输入 `yes` 执行；
- 输入 `no` 拒绝；
- 高风险命令使用红色醒目标记；
- 长命令自动折行，也可以展开查看完整内容。

你始终知道即将执行什么，也始终拥有最后决定权。

### 📊 把监控台放进 SSH

右上角可以同时展示系统、进程、磁盘、网络、服务和自定义监控。

- 支持单屏、双栏、`1×3`、`2×2` 和自适应网格；
- 每个分屏都能独立选择显示内容；
- 用户可以手动添加监控；
- AI 在分析过程中也可以按需打开或更新监控面板。

小屏幕专注操作，大屏幕直接变成一套轻量运维仪表盘。

### 🔗 同时理解本地代码与线上环境

使用本机 Codex 时，`aissh` 可以只读查看本地项目，再结合服务器输出进行分析。

```bash
cd /path/to/local/project
aissh production
```

连接后也可以随时切换本机工作区：

```text
/workspace                    # 查看当前工作区
/workspace /path/to/project   # 切换工作区
/workspace off                # 关闭本机工作区访问
```

这些命令只在本机处理，不会发送到目标服务器。工作区切换只影响后续 AI 请求，也不会自动上传、
修改或部署本地代码。

### 💬 每台服务器都有自己的 AI 记忆

AI 对话按 SSH 目标保存在本机。点击右下角 `[会话]`，可以：

- 查看这台服务器的历史会话；
- 回到之前的排查上下文继续提问；
- 随时创建一个干净的新会话。

重复问题不必每次从头说明，长期维护同一批服务器会更顺手。

### 🖱️ 像桌面应用一样操作终端

面板、标签、审批按钮和历史会话都支持鼠标点击。SSH、监控和 AI 内容可以直接拖动选择，
普通右键复制；各区域也拥有独立滚动能力。

键盘用户仍然可以完成所有核心操作。

## 适合这些场景

- 登录服务器后快速定位服务异常；
- 一边执行命令，一边观察 CPU、内存、日志和连接状态；
- 将本地源码与线上日志放在同一个分析上下文中；
- 让 AI 给出操作建议，但保留人工审批和风险控制；
- 在不改造目标服务器的前提下，为现有 SSH 工作流加入 AI；
- 维护多台服务器，并持续保留各自主机的排查历史。

## 快速开始

目前可以直接从源码构建，需要 Go 1.23 或更高版本：

```bash
go test ./...
go build -o bin/aissh ./cmd/aissh
```

默认使用本机 Codex。先确认 Codex 已登录：

```bash
codex exec --ephemeral "只回答 ok"
```

然后连接服务器：

```bash
bin/aissh production
```

如需指定 Codex 路径或模型：

```bash
export AISSH_CODEX_PATH='/usr/local/bin/codex'
export AISSH_CODEX_MODEL='gpt-5.2'
bin/aissh production
```

## 选择 AI 后端

### 本机 Codex（默认）

适合需要结合本地代码进行分析的场景。复用本机 Codex 登录和配置，目标服务器不安装任何 AI 组件。

```bash
bin/aissh --agent codex production
```

### DeepSeek

不需要安装 Codex CLI，配置 API Key 后即可使用：

```bash
export DEEPSEEK_API_KEY='...'
bin/aissh --agent deepseek production
```

### 其他 OpenAI-compatible 模型

```bash
export AISSH_MODEL_URL='https://model.example/v1'
export AISSH_MODEL_API_KEY='...'
export AISSH_MODEL_NAME='your-model'
bin/aissh --agent model production
```

直连模型可以使用 AI 对话、命令审批、结果复核和监控面板，但不会读取本机代码工作区。

## 常用操作

| 操作 | 功能 |
| --- | --- |
| 中文问题 / `/ai 问题` | 在当前 SSH 提示符向 AI 提问 |
| `/workspace 路径` | 切换本机 Codex 工作区 |
| `yes` / `no` | 执行或拒绝 AI 命令建议 |
| `Ctrl+O` | 展开当前折叠的完整命令 |
| `Ctrl+R` | 在 AI 对话中浏览历史会话 |
| `Shift+Enter` / `Ctrl+J` | 在 AI 输入框中换行 |
| `Ctrl+T` | 切换监控 Tab |
| `Ctrl+L` | 切换监控布局 |
| `Ctrl+N` | 新增监控 Tab |
| `Esc` | 返回 SSH 主终端 |

`rz` / `sz` 文件传输也可以继续使用，传输期间界面会自动让出终端控制，结束后恢复工作区。

## 常用配置

```bash
# 指定本机项目
bin/aissh --workspace /path/to/project production

# 禁止 AI 查看本机项目
bin/aissh --no-workspace production

# 切换界面语言
bin/aissh --lang en production

# 调整 AI 请求超时
bin/aissh --ai-timeout 10m production

# 调整 SSH 历史保留行数
bin/aissh --scrollback-lines 50000 production

# 临时关闭本机 AI 历史
bin/aissh --no-history production
```

更多参数请运行：

```bash
bin/aissh --help
```

## 安全设计

- 目标服务器无需安装任何组件；
- AI 命令建议必须由用户明确批准；
- 高风险命令在左右两侧都会醒目标记；
- 本机 Codex 仅以只读方式查看工作区；
- AI 创建的监控仅允许使用只读命令；
- 用户创建的监控默认不会交给 AI 分析；
- 本地代码不会自动修改、上传或部署；
- 登录信息、API Key 和本机历史不会发送到目标服务器。

使用第三方模型服务时，本次问题、相关 SSH 输出和命令执行结果会发送给所配置的模型 API，
请根据实际数据安全要求选择后端。

## Roadmap

- 第二个交互式 Shell；
- 可保存和复用的监控布局；
- 更丰富的流式日志面板；
- 对话与排查报告导出；
- 更多服务器和网络设备连接方式。

---

`aissh` 的目标不是替代 SSH，而是让你在不改变现有基础设施的情况下，获得一个更聪明、
更可视、也更可控的远程工作环境。
