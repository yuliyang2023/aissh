# 桌面客户端与凭据管理

## 从哪里开始

macOS 构建产物是 `dist/desktop/WRD AI SSH.app`。可直接双击，或复制到「应用程序」。首次运行创建空的本机配置，不导入或扫描你的密码。

1. 在「凭据」中添加账号密码、SSH 私钥引用、私钥口令、SSH Agent 或 API Token。
2. 在「主机」中填写地址、端口和账号，选择登录凭据；需要跳板机时先添加跳板机，再绑定到目标主机。
3. 点击「测试连接」或「连接」。首次连接显示主机指纹，核对后才能使用保存的凭据。
4. 在「AI 配置」中选择本机 CLI，或填写模型 API 地址、模型名称和 Token 凭据。可设为默认，也可在每台主机中单独选择。
5. 在「系统连接」中添加 API、数据库或 Redis，绑定账号、关联主机和允许的操作范围。

界面采用统一的工作空间侧栏、独立搜索栏和浅色 / 深色外观，跟随系统切换。主机支持按直接连接 / 通过跳板筛选，可与分组和搜索组合使用。macOS 保留系统菜单栏和延伸至标题栏的侧栏；可用 `⌘N` 新建主机、`⌘⇧N` 新建凭据、`⌘1–5` 切换页面、`⌘⌥S` 显示或隐藏侧栏，以及 `⌘F` 搜索配置。Windows / Linux 使用 Ctrl 修饰键。表单打开时，菜单不会打断当前录入。

登录后沿用 aissh 的 `/ai`、中文提问、命令确认、监控和聊天记录。侧栏「会话管理」（`⌘5` / `Ctrl+5`）集中显示当前会话和最近连接：

- 当前会话显示主机、账号、地址、开始时间、时长，以及正在启动、运行中、已结束或异常结束状态；支持搜索和状态筛选。
- 可切换到终端、确认断开运行中的连接、重新连接已结束的会话，或一键清理已结束的标签页。切换页面不会重启终端。
- 最多同时运行 12 个会话，已结束或启动失败的标签页不占运行名额；最多保留 50 个终端标签。
- 最近打开过的 50 台主机会保存在本机配置中，按主机去重，可再次连接或清空记录。删除主机也会移除其最近记录。

「运行中」表示本机终端工作台已启动，SSH 认证及远端连接结果以终端输出为准。重连会使用当前保存的主机配置并重新检查主机信任；这是一次新的 SSH 连接。退出客户端会终止当前连接，不会自动恢复远端进程。最近记录仅保存主机引用和打开时间，不保存终端内容、密码或 AI 对话（AI 聊天历史仍由 aissh 原有功能管理）。

本机 Codex / Claude Code 使用其原有登录状态。若从 Finder 启动时找不到 CLI，可在 AI 配置中填写 CLI 的绝对路径。模型 API Key 通过「兼容 OpenAI API 的模型」或「DeepSeek」配置并保存在凭据库。HTTP Agent 也支持绑定 Bearer Token。

桌面主机未设置「本机工作区」时关闭本机文件访问；选择目录后沿用 aissh 默认只读工作区策略。已有命令行用法及 `--workspace-write` 保持可用。

## 跨平台分层

沿用 Wails 桌面壳，界面资源随应用本地打包。React 负责导航、列表、搜索、表单和终端显示；Go 服务负责配置校验、授权、凭据读取、SSH 会话与隧道、系统适配器和本机 AI 进程；系统接口负责窗口、菜单、文件选择和各平台凭据库。业务界面在三个平台复用，标题栏与快捷键遵循平台习惯。

录入秘密时，表单短暂持有用户输入并通过受限服务方法提交；读取配置只返回元数据与 `hasSecret` 状态，不向界面返回已保存的秘密，也不提供通用的秘密读取接口。连接、资源访问和 AI 调用都由 Go 层按配置引用取用凭据。后续增加通知、托盘等系统联动时，放在平台适配层，避免业务界面依赖某一操作系统。

界面跨平台复用不代表系统集成已经在各平台完成验收；Windows / Linux 仍需要原生运行环境检查窗口、SSH、凭据库及打包行为。

## 系统连接怎样使用凭据

AI 得到系统名称、类型和操作 ID，提出结构化操作；aissh 根据配置和批准状态取出凭据，再执行对应适配器。秘密不会拼入 AI 生成的 Shell 命令，也不会写到 SSH 主机上的临时文件。

| 类型 | 首版支持 | 认证 |
| --- | --- | --- |
| HTTP API | 固定服务地址下的 GET，禁止自动重定向 | Basic / Bearer |
| PostgreSQL | 只读事务查询，最多 100 行 | 用户名 / 密码 |
| MySQL | 只读事务查询，禁用多语句，最多 100 行 | 用户名 / 密码 |
| Redis | PING、INFO、DBSIZE、GET、TTL、PTTL、TYPE、STRLEN、SCARD、LLEN、HLEN、ZCARD | 密码或 ACL 用户名 / 密码 |

响应大小限制约 128 KiB，操作有 30 秒总超时。数据库应使用服务端已限制权限的只读账号：只读事务不能代替服务端权限，HTTP GET 的业务副作用也取决于服务自身。

访问路径可以选「本机直接访问」或「通过关联主机的 SSH 隧道」。隧道使用 OpenSSH `direct-tcpip`，不开放本机监听端口，不在远端安装代理；SSH 服务端需要允许 TCP 转发。TLS 校验的是目标系统的主机名。数据库和 Redis 可按部署情况关闭 TLS；SSH 到目标系统之间是否加密由这个选项决定。

授权有三种：

- **每次操作确认**：每次提出操作都需要确认。
- **首次确认后，本次会话允许**：当前 aissh 进程批准一次；连接配置或凭据变更后重新确认。
- **记住此范围，自动执行只读操作**：允许适配器支持的读取操作，无需重复输入密码或确认。

系统关联的 SSH 主机限定它在何处可用。「同时允许本机工具使用」是单独开关。`--always-yes` 不会绕过系统自身的授权策略。桌面「执行」按钮只批准本次操作；不会给其他进程授予会话权限。

## 命令行与本机工具

桌面端和 CLI 使用同一份配置。配置 ID 可以从 `connections.json` 中读取，它只含元数据。

```bash
# 使用保存的主机和 AI 配置
aissh --profile HOST_ID
aissh --profile HOST_ID --ai-profile AI_PROFILE_ID

# 本机工具访问已授权系统，返回经过已知秘密脱敏的结果
aissh --resource '{"resourceId":"RESOURCE_ID","action":"http.get","path":"/health"}'
aissh --resource '{"resourceId":"RESOURCE_ID","action":"database.query","query":"SELECT 1"}'
aissh --resource '{"resourceId":"RESOURCE_ID","action":"redis.read","args":["INFO"]}'
```

非交互本机工具需要同时开启「允许本机工具使用」和持久只读授权。它不能通过 JSON 传入批准标志、目标 URL、另一台 SSH 主机或任意请求头。需要逐次审批时，在桌面或交互式 aissh 中操作。

`--profile` 不接受额外 SSH 地址、参数或 `--ssh-path` 覆盖；保存的 AI 配置会同时决定服务地址和凭据，避免地址被命令行覆盖后把秘密发送到另一个服务。原有 `aissh user@host` 仍使用原生 SSH 参数；配置了默认 AI 后端后也会复用该配置。

## 保存、更新与恢复

| 数据 | 位置 |
| --- | --- |
| macOS 连接配置 | `~/Library/Application Support/aissh/connections.json` |
| Linux 连接配置 | `$XDG_CONFIG_HOME/aissh/connections.json`，默认 `~/.config/aissh/` |
| Windows 连接配置 | `%AppData%\aissh\connections.json` |
| 主机信任记录 | 同目录 `known_hosts`；也读取用户已有的 SSH 主机信任记录 |
| 密码 / Token / 私钥口令 | macOS Keychain、Windows Credential Manager、Linux Secret Service |
| SSH 私钥 | 原有文件或 SSH Agent，客户端只保存引用 |

`AISSH_CONFIG_DIR` 可以指定独立配置目录。OS 凭据服务标识是 `com.wrdtech.aissh.credentials`。配置通过文件锁和原子替换保存；Unix 下目录权限 0700、文件 0600。Windows 下沿用用户配置目录的 ACL。

编辑已有凭据时，秘密字段留空表示保留，输入新值表示轮换。删除仍被引用的凭据会被阻止。轮换时先保存新秘密和引用，再清理旧项；系统凭据库锁定导致的清理失败会显示「清理旧凭据」入口。

桌面连接进行中，如果已保存的秘密不可读取，会弹出凭据更新窗口；更新或解锁后继续原任务。密码被服务器拒绝时不会反复提交旧密码，请编辑凭据后重新连接。MFA / keyboard-interactive 不自动代填。

配置文件备份不包含密码；迁移到其他设备后需要重录秘密并重新选择私钥。首版不提供云同步、团队共享、密码明文导出或自建主密码保险库。

主机公钥变更时 OpenSSH 会拒绝连接。请先通过可信渠道核对变更，再手动更新对应 `known_hosts` 条目；客户端不会自动覆盖旧指纹。SSH 配置导入目前只导入名称、地址、端口、用户，导入后需核对私钥和跳板机绑定。

AI 请求、aissh 聊天记录和适配器返回值会过滤当前已读取的秘密及常见编码形式。这不能识别所有未知秘密或任意编码；远端输出和本机 CLI 自身的记录仍遵循各自的数据处理规则。OS 凭据库也不隔离已经取得当前用户权限的恶意程序。

## 构建与验证

需要 Go 1.25+、Node.js 20+、npm。首个构建下载 Go 和 npm 依赖。

```bash
go test ./...
make desktop
```

- macOS：需要 Xcode 命令行工具。构建生成当前架构 `.app`，内含配套 CLI，并使用本机临时签名；面向其他用户分发前需要正式签名和公证。
- Windows：运行 `powershell -File scripts/build-desktop.ps1`，保持两个 `.exe` 在同一目录。需要系统 OpenSSH Client 和 WebView2 Runtime。
- Linux：需要 GTK3 和 WebKitGTK 开发包、图形会话与已解锁 Secret Service。发行版提供 WebKitGTK 4.1 时，可通过 `GOFLAGS=-tags=webkit2_41` 构建。
- 安装过前端依赖后，可以设置 `AISSH_SKIP_INSTALL=1` 跳过 `npm ci`。`GO`、`NPM` 支持指定工具路径（Windows 脚本使用 PATH 中的 npm）。

```bash
# 竞态与回归测试；SSH 集成测试需要系统 ssh 和本机临时端口
go test -race -p 2 ./...

# 实测 OS 凭据库，只操作独立测试服务中的随机条目并自动删除
AISSH_TEST_OS_KEYRING=1 go test ./internal/catalog -run TestOSKeyringRoundTrip -v

# 界面流程测试：需先构建 frontend，并安装 Playwright 和 Chrome
# 可通过 PLAYWRIGHT_MODULE / CHROME_PATH 指定现有安装
node scripts/test-desktop-ui.mjs
```

本次开发验证了 macOS 原生窗口、真实 Keychain、真实 OpenSSH 密码/加密私钥认证、跳板机与隧道、授权和脱敏，以及浏览器中的完整界面操作。界面自动化使用内存 Wails bridge，真实窗口启动另行验证。Windows/Linux 的桌面交互及你的实际业务数据库仍需在对应环境验收。
