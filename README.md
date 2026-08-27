# WRD AI SSH

**English** | [简体中文](README.zh-CN.md)

> Bring an AI assistant, live observability, and human-controlled execution into the SSH workflow you already know.

`aissh` is a local-first AI workspace for SSH. It keeps the native OpenSSH experience while bringing troubleshooting,
command proposals, live monitoring, and AI conversations together in one terminal UI.

Nothing needs to be installed on the target server—no agent, no script, and no resident service. Connect as usual and
start working.

```text
┌──────────────────── SSH Terminal ────────────────────┬──── Live Monitoring ────┐
│                                                     │ [System] [Process] [Disk]│
│ root@server:~# /ai find out why nginx is slowing down │                        │
│ AI ⠹ Analyzing server state and local code…           ├──── AI Conversation ────┤
│ AI: Upstream connections are timing out. Start with   │ Full context and review │
│     the error log and current connection count.       │ history                 │
└───────────────────────────────────────────────────────┴─────────────────────────┘
```

## Upgrade SSH with one command

There is no new connection workflow to learn:

```bash
aissh root@192.0.2.1
aissh -p 2222 production
aissh -J bastion internal-server
```

Your existing `~/.ssh/config`, keys, SSH agent, jump hosts, and OpenSSH options continue to work.

## Core experience

### ✦ AI answers where you are working

Ask a question directly from the SSH prompt and the answer appears immediately below it, without moving you away from
the current terminal context.

```text
root@server:~# /ai check nginx for recent errors
AI: Nginx is running, but several upstream timeouts occurred in the last 10 minutes…
```

Chinese questions can be entered directly. For English and other languages, use `/ai`:

```text
/ai check why this service keeps restarting
```

The left pane is optimized for handling the issue at hand. The bottom-right conversation keeps the full context,
history, and review trail. Both views stay synchronized.

### ⚡ Proactive failure analysis

When common errors such as `command not found` appear, `aissh` can automatically analyze the failed command together
with recent terminal output. There is no need to copy logs into another window and explain the situation again.

After an approved command completes, AI can review its output and help determine whether the issue is actually resolved.
While a long AI turn is running, `aissh` also keeps the SSH transport and an idle shell prompt active to prevent
automatic logout.

### 🛡️ AI proposes, you decide

AI-generated commands are never executed automatically. Each proposal is shown in the SSH pane with its purpose,
expected effect, and risk level.

- Enter `yes` to run it.
- Enter `no` to reject it.
- High-risk commands are highlighted in red.
- Long commands are formatted for readability and can be expanded in place.
- Approved commands run in an isolated child shell on the same SSH connection, so shell-local control flow such as
  `exit`, `exec`, `set -e`, or `cd` cannot terminate or mutate the outer login shell.

You always know what is about to run, and you always make the final decision.

### 📊 A monitoring workspace inside SSH

The upper-right workspace can display system health, processes, disks, networking, services, and custom monitors at the
same time.

- Single, split, `1×3`, `2×2`, and adaptive grid layouts
- Independent content selection for every tile
- User-created monitor tabs
- Monitoring panels that AI can open or update while investigating

Stay focused on a small screen, or turn a large terminal into a lightweight operations dashboard.

### 🔗 Connect local code with the remote environment

With the local Codex or Claude Code provider, `aissh` can inspect a local project in read-only mode and analyze it
together with output from the remote server.

```bash
cd /path/to/local/project
aissh production
```

The local workspace can also be changed after the SSH connection is established:

```text
/workspace                    # Show the current workspace
/workspace /path/to/project   # Change the workspace
/workspace off                # Disable local workspace access
```

These commands are handled locally and are never sent to the target server. A workspace change affects subsequent AI
requests only; local code is never uploaded, modified, or deployed automatically.

### 💬 A separate AI memory for every server

Conversation history is stored locally and grouped by SSH target. Use `[Sessions]` in the bottom-right pane to:

- Browse previous conversations for the current server
- Continue an earlier investigation with its context
- Start a clean conversation at any time

Recurring problems no longer need to be explained from scratch every time.

### 🖱️ Terminal speed with desktop-style interaction

Panels, tabs, approval buttons, and sessions can all be operated with the mouse. Drag to select text across SSH,
monitoring, and AI panes, then right-click to copy. Each area also has its own scrolling behavior.

Keyboard-first users can still access every core feature.

## Built for

- Diagnosing service failures immediately after connecting to a server
- Watching CPU, memory, logs, and connections while running commands
- Analyzing local source code and production output in the same context
- Getting AI-assisted command proposals without giving up human approval
- Adding AI to an existing SSH workflow without changing server infrastructure
- Maintaining per-host troubleshooting history across a fleet of servers

## Quick start

The project can currently be built from source and requires Go 1.23 or newer:

```bash
go test ./...
go build -o bin/aissh ./cmd/aissh
```

Local Codex is the default AI provider. First, make sure Codex is authenticated:

```bash
codex exec --ephemeral "reply with ok only"
```

Then connect to a server:

```bash
bin/aissh production
```

To select a Codex executable or model:

```bash
export AISSH_CODEX_PATH='/usr/local/bin/codex'
export AISSH_CODEX_MODEL='gpt-5.2'
bin/aissh production
```

Claude Code is also supported as a local provider; see “Choose an AI provider” below.

## Release builds and GitLab distribution

The included GitLab pipeline tests the project and builds Linux/macOS packages for AMD64 and ARM64. It uses a pinned
Go container image, so a Docker or Kubernetes GitLab Runner does not need Go installed on the host. Branch packages
are retained as job artifacts for seven days. Tags matching `vX.Y.Z` are also uploaded to the Generic Package Registry
and exposed through a GitLab Release.

Go jobs use `https://goproxy.cn,direct` for module downloads and `sum.golang.google.cn` for checksum verification by
default. To use an internal mirror, define the `GOPROXY` and, if necessary, `GOSUMDB` variables in the project's CI/CD
settings; project variables override these pipeline defaults.

Create a release by pushing a version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

For a Shell Runner without Go or container support, set the project CI/CD variable `AISSH_LOCAL_RELEASE=true` to skip
the container-based jobs, then build and publish from a development machine instead:

```bash
make dist VERSION=v1.0.0

git tag v1.0.0
git push origin v1.0.0

export GITLAB_API_URL='https://git.example.com/api/v4'
export GITLAB_PROJECT_ID='123'
export GITLAB_TOKEN='glpat-...'
make publish VERSION=v1.0.0
```

The tag must already exist in GitLab before `make publish` creates the Release. The personal access token needs API
access to the project. `dist/` contains the four archives and `SHA256SUMS`; it is intentionally excluded from Git.

## Choose an AI provider

### Local Codex (default)

Best when investigations should include local source code. It reuses your local Codex authentication and configuration;
no AI component is installed on the server.
Each running SSH session owns one local Codex thread. Follow-up questions, command approvals, and execution results
resume that thread instead of creating a new agent. The thread is removed when the SSH session ends, while aissh's
local text history remains available.

```bash
bin/aissh --agent codex production
```

### Local Claude Code

The same local-first experience backed by the Claude Code CLI, including read-only access to the local code workspace.
Make sure the `claude` CLI is installed and authenticated:

```bash
claude -p "reply with ok only"
```

Then connect with:

```bash
bin/aissh --agent claude production
```

To select a Claude executable or model:

```bash
export AISSH_CLAUDE_PATH='/usr/local/bin/claude'
export AISSH_CLAUDE_MODEL='opus'
bin/aissh --agent claude production
```

Each running SSH session owns one persistent Claude session that is resumed across follow-up questions, approvals, and
result reviews. Claude runs with a hard read-only tool allowlist; with `--no-workspace`, local file access is disabled
entirely. Note that the Claude Code CLI keeps its own local session transcripts and provides no delete command, so
unlike Codex threads they are not removed when the SSH session ends.

### DeepSeek

No Codex CLI is required. Configure an API key and connect:

```bash
export DEEPSEEK_API_KEY='...'
bin/aissh --agent deepseek production
```

### Other OpenAI-compatible models

```bash
export AISSH_MODEL_URL='https://model.example/v1'
export AISSH_MODEL_API_KEY='...'
export AISSH_MODEL_NAME='your-model'
bin/aissh --agent model production
```

Direct model providers support AI conversations, command approval, result review, and monitoring panels, but they do not
receive access to the local code workspace.

## Everyday controls

| Input | Action |
| --- | --- |
| Chinese question / `/ai question` | Ask AI from the current SSH prompt |
| `/workspace PATH` | Change the local AI code workspace |
| `yes` / `no` | Run or reject an AI command proposal |
| `Ctrl+O` | Expand the currently folded command |
| `Ctrl+R` | Browse sessions while the AI conversation is focused |
| `Shift+Enter` / `Ctrl+J` | Insert a new line in the AI editor |
| `Ctrl+T` | Switch monitoring tabs |
| `Ctrl+L` | Change the monitoring layout |
| `Ctrl+N` | Add a monitoring tab |
| `Esc` | Return to the main SSH terminal |

`rz` and `sz` file transfers continue to work. The workspace temporarily yields terminal control during a transfer and
restores the full UI afterward.

## Common configuration

```bash
# Select a local project
bin/aissh --workspace /path/to/project production

# Prevent AI from inspecting any local project
bin/aissh --no-workspace production

# Select the interface language
bin/aissh --lang zh-CN production

# Increase the AI request timeout
bin/aissh --ai-timeout 10m production

# Change the keepalive interval used while AI is running (default: 30s)
bin/aissh --ai-keepalive 20s production

# Disable AI-time keepalive
bin/aissh --ai-keepalive off production

# Keep more SSH scrollback lines
bin/aissh --scrollback-lines 50000 production

# Disable local AI history for this connection
bin/aissh --no-history production

# Record OpenSSH protocol diagnostics (useful when the session ends with EOF)
bin/aissh -vvv -E /tmp/aissh-ssh.log production
```

Run the following command for the complete option reference:

```bash
bin/aissh --help
```

## Safety by design

- Nothing needs to be installed on the target server.
- Every AI command proposal requires explicit user approval.
- High-risk commands are clearly highlighted in both views.
- Local Codex and Claude Code can inspect the configured workspace in read-only mode only.
- AI-created monitors are restricted to read-only commands.
- User-created monitor data is not shared with AI by default.
- Local code is never modified, uploaded, or deployed automatically.
- Authentication data, API keys, and local history are never sent to the target server.

When a third-party model provider is used, the question, relevant SSH output, and approved command results are sent to
the configured model API. Choose a provider that matches your data security requirements.

## Roadmap

- A second interactive shell
- Reusable and persistent monitoring layouts
- Richer streaming log panels
- Conversation and incident-report export
- More server and network-device connection modes

---

`aissh` is not trying to replace SSH. Its goal is to make the remote environment you already use smarter, more visible,
and more controllable—without requiring changes to your infrastructure.
