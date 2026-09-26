# 实时监控数据管线 v2：先试采，再映射，再展示

状态：首版已实现，2026-09-26。已通过模拟 SSH 全链路、数据执行器、Chromium/WebKit 和原生 Wails 渲染测试；真实服务器环境仍需使用新版客户端首采验证。

所有 `probe` 首采须先经过用户命令审计。各数据源分别展示完整命令和采样周期，批准后在当前 SSH 终端执行并显示输出及退出码；周期采样复用相同配置的授权，命令或周期改变后重新审计。人工审阅时间不计入采集等待超时，拒绝会停止该批采集；恢复历史组件也需重新审计。

本实现修订 v1 的采集与生效流程，复用 TSX 编译、QuickJS 隔离和宿主渲染。桌面向 AI 声明 `monitorWidgetV2`，通过客户端原生回执自动串联 `probe → map → create/update`，并在真实数据渲染通过后激活。

## 1. 核心约定

**第一次在当前 SSH 会话里实际执行采集命令，取得真实输出，是创建面板的前置条件。**

数据链路固定为：

```mermaid
flowchart LR
    S[当前 SSH 会话执行命令] --> C[客户端封装原始采样]
    C --> P[本地解析器]
    P --> M[本地组装与字段映射]
    M --> D[统一数据集与历史]
    D --> B[组件数据绑定]
    B --> V[自定义组件与整体布局]
```

AI 根据首采结果设计解析规则、映射、组件和布局。普通刷新由固定执行器完成，不再调用 AI。Shell 只需输出命令原有的文本；JSON 封装、字段转换、聚合、速率和图表数据均在客户端处理。服务本身已有 JSON 接口时可以直接读取，再由本地 JSON 解析器处理。

这里的客户端是运行桌面应用的本机，采集命令仍执行在绑定的 SSH 目标上；不能为了解决依赖问题，悄悄改成查询客户端自己的进程、端口或数据库。

## 2. 用户与 AI 的实际流程

1. 用户描述监控目标。
2. AI 提出只读取数命令，通过当前 SSH 执行通道**实际试采**。工具、参数、权限或接口不可用时，先调整命令并重新试采。
3. 客户端返回原始结果、执行状态和首采凭据。AI 据此确认实际列、字段、单位和缺失情况。
4. AI 提交解析与映射规则；本地执行器用刚才保存的完整采样校验规则，返回统一数据集或具体映射错误。
5. AI 根据这个数据集生成组件、数据绑定和整体展示方案。
6. 客户端用真实数据验证整个展示链路，再检查空值、失败和过期状态。全部通过后原子生效，开始按相同配置定时采集。

采集失败停在第 2 步；字段解释失败停在第 4 步；渲染失败停在第 6 步。不能先显示“已生效”，再发现连第一次采集都无法完成。

命令执行沿用当前 SSH 通道的只读校验与既有授权规则。创建试采走正式采集的同一执行接口，禁止出现“试采是一条命令，定时执行是另一条命令”的差异。支持专用 SSH 通道和已认证的主会话通道。

## 3. 三类对象

### 3.1 SourceSpec：要执行什么

```ts
type SourceSpec = {
  id: string;
  target: { kind: 'ssh'; sessionId: string; generation: number };
  command: string;
  timeoutMs: number;
  required: boolean;
};
```

目标、工作目录和环境继承规则由当前会话执行接口明确返回。命令必须自包含，不依赖上一次执行留下的 `cd`、变量或临时文件。一个数据源对应一条独立命令；Nginx 的进程、服务、端口可以是三个数据源，无需拼成一个远端 Python 程序。

`required` 表示该源是否为用户要求的核心指标所必需。必需源首采失败时不激活面板；可选源失败必须在数据集中保留未知状态和诊断，不能通过把所有源标为可选绕过创建验证。

### 3.2 Capture：Shell 输出的统一封装

以下字段由客户端生成，AI 不能自行提交一份 Capture 充当真实采样：

```ts
type Capture = {
  captureId: string;
  sourceId: string;
  sessionId: string;
  sessionGeneration: number;
  commandHash: string;
  sourceRevision: number;
  sequence: number;
  startedAt: string;
  finishedAt: string;
  durationMs: number;
  exitCode: number | null;
  transport: 'ssh-exec' | 'ssh-primary';
  streams: 'separate' | 'merged';
  stdout: string;
  stderr: string | null;
  status: 'ok' | 'exit_error' | 'timeout' | 'disconnected' | 'too_large';
  complete: boolean;
};
```

这只是客户端的封装格式，不要求 Shell 输出该 JSON。专用通道保留 stdout/stderr；主会话无法分离时明确声明 `streams=merged`、`stderr=null`，不能伪造流的来源。缺少退出状态、输出超限、断线或超时的记录不能作为完整成功样本。

AI 得到经过脱敏的采样预览与凭据；完整的有界采样由客户端持有。聊天中截短的输出只用于说明，映射验证必须读取实际保存的完整采样。样本被截断时不能把可见行数当成真实总数。

### 3.3 MonitorPlan：AI 生成的可保存方案

```ts
type MonitorPlan = {
  protocolVersion: 2;
  id: string;
  title: string;
  expectedRevision: number;
  intervalMs: number;
  sources: { sourceId: string; captureId: string }[];
  dataPlan: {
    parsers: ParserSpec[];
    steps: TransformSpec[];
    outputs: OutputSpec[];
  };
  presentation: {
    bindings: BindingSpec[];
    view: { kind: 'custom'; sdkVersion: 2; source: string };
  };
};
```

创建请求引用客户端发放的首采凭据。客户端核对目标、会话代次、命令摘要和来源版本，再解析数据；修改命令或换连接后必须重新试采。一个凭据最多用于同一候选方案的修正，不允许跨面板、跨目标复用。

保存时将凭据解析为真实 SourceSpec，保存命令、数据计划、绑定和组件；不持久化原始样本或首采凭据。重新打开“监控历史组件”时，仍先在当前连接试采。

## 4. 本地解析、组装和映射

执行器采用有类型、有限操作集的声明式数据计划。AI 选择操作并填写规则，本地执行器负责含义和错误处理。

| 层 | 第一版操作 | 输出 |
| --- | --- | --- |
| 解析 | `lines`、`kv`、`delimited`、`regex_rows`、`json` | 有类型的记录或表 |
| 筛选 | `filter`：等于、范围、枚举、RE2 匹配 | 保持列类型的表 |
| 组装 | `project`、`rename`、`concat`、显式键的 `join` | 已声明字段的表 |
| 汇总 | `count`、`sum`、`min`、`max`、`avg` | 标量 |
| 换算 | 类型转换、单位换算、`add/subtract/multiply/divide`、枚举映射 | 有类型的值 |
| 时间 | `delta`、`rate`、滚动窗口 | 本地历史上的派生指标 |
| 展示准备 | `sort`、`limit` | 带总数、筛选与省略说明的表 |

引用使用节点 ID 和字段 ID；运算写成结构化操作，不使用任意 JavaScript、Shell 或字符串 `eval`。执行器先检查引用存在、无环、类型和单位兼容，再按依赖顺序运行。新增通用算子通过客户端版本升级提供，不为每项服务增加专用采集程序。

解析器明确空行、表头、分隔符、引号及不匹配行的处理。默认遇到不能解释的业务行就报告解析失败；只有规则明确允许忽略的表头/空行可以跳过。`ps` 命令的列格式不能靠组件临时猜测。

首版不隐式拼接不同时间的数据。跨源 join 必须声明连接键与最大采样时间差；超出时间差或缺少源时输出未知/不完整。独立指标可以分别更新，并保留各自的采样时间。

### Nginx 进程示例

先在目标 SSH 下试运行：

```sh
LC_ALL=C ps -e -o pid= -o comm= -o rss=
```

下面是**说明协议用的测试样本**，不代表任何真实主机：

```text
1 init 1024
1823 nginx 20480
1824 nginx 10240
2000 other process 4096
```

AI 看到实际格式后，可以提交以下数据计划；正则支持中间进程名含空格：

```json
{
  "parsers": [{
    "id": "processes",
    "sourceId": "ps",
    "kind": "regex_rows",
    "pattern": "^\\s*(?P<pid>[0-9]+)\\s+(?P<name>.+?)\\s+(?P<rssKiB>[0-9]+)\\s*$",
    "columns": [
      {"id": "pid", "type": "string"},
      {"id": "name", "type": "string"},
      {"id": "rssKiB", "type": "number", "unit": "KiB"}
    ],
    "skipBlankLines": true,
    "onUnmatched": "error"
  }],
  "steps": [
    {"id": "nginx", "op": "filter", "input": "processes", "field": "name", "test": "in", "values": ["nginx", "nginx-debug"]},
    {"id": "count", "op": "count", "input": "nginx"},
    {"id": "rss", "op": "sum", "input": "nginx", "field": "rssKiB"},
    {"id": "memory", "op": "convert_unit", "input": "rss", "from": "KiB", "to": "MiB"}
  ],
  "outputs": [
    {"id": "processCount", "kind": "metric", "input": "count", "unit": "个", "history": true},
    {"id": "rssMiB", "kind": "metric", "input": "memory", "unit": "MiB", "history": true},
    {"id": "processList", "kind": "table", "input": "nginx"}
  ]
}
```

这份计划在客户端得出进程数 2、RSS 合计 30 MiB 和进程表。服务状态、监听信息同样采用独立首采、标准解析和映射，再通过 presentation 组合展示。RSS 合计的说明必须指出共享页可能重复计数；不能将其标成服务独占内存。

## 5. 统一数据集与组件绑定

```ts
type Quality = 'ok' | 'partial' | 'unknown' | 'stale';
type Metric = {
  value: number | string | boolean | null;
  unit: string;
  quality: Quality;
  observedAt: string | null;
  sourceIds: string[];
  reason?: string;
};
type Table = {
  columns: {id: string; type: string; unit?: string}[];
  rows: Record<string, unknown>[] | null;
  quality: Quality;
  observedAt: string | null;
  sourceIds: string[];
  totalRows: number | null;
  shownRows: number;
  omittedRows: number | null;
  reason?: string;
};
type Dataset = {
  definitionRevision: number;
  dataRevision: number;
  sequence: number;
  metrics: Record<string, Metric>;
  tables: Record<string, Table>;
  diagnostics: {stage: string; sourceId?: string; nodeId?: string; message: string}[];
};
```

示例绑定：

```json
[
  {"prop": "count", "kind": "metric", "outputId": "processCount"},
  {"prop": "memory", "kind": "metric", "outputId": "rssMiB"},
  {"prop": "processes", "kind": "table", "outputId": "processList"}
]
```

组件读取 `data.count.value`、`data.memory.value`、`data.processes.rows`，通过 `history.series('memory')` 读取本地已经计算好的历史。卡片位置、颜色、图表、表格和整体布局由 AI 编写的组件决定。组件只消费绑定后的数据，不解析原始 Shell、不算跨采样计数、不发起命令。新字段类型与绑定在发布前校验。

客户端在组件外显示阶段错误、数据更新时间与质量；组件可以进一步展示局部字段状态。外层保证“未知”的语义不会因某个组件漏写提示而完全消失。

## 6. 统一计算语义

- 成功且完整的进程表经筛选没有 Nginx，计数可以是 0；命令失败、权限不足、输出截断或解析失败时计数必须为 null。
- `sum` 对已确认的完整空集合返回 0；缺失字段、不完整集合默认输出 null，部分汇总只有显式声明时才允许，并标记 partial。
- null 参与普通运算得到 null；除数为零、非有限值或超过安全整数范围得到未知与诊断。大整数保持字符串，不强行转浮点。
- 单位转换由执行器检查，例如 KiB → MiB，不能只改标签。字段为 counter/gauge 的语义由计划声明并验证。
- 速率至少需要两个真实有效样本；使用客户端采集时间和单调时钟计算间隔。首次只有一个点时，卡片显示等待采样。
- 失败点、断线、计数回退和来源版本变化产生历史缺口，不补零。计数器重置或服务重启后重新建立基线；只有已声明位宽且可证实的回绕才允许修正。
- 旧成功值单独保留时间与 stale 状态，不能冒充本次值。源、字段解释或单位变化后不能将旧历史拼进新指标。

## 7. 生效事务与防止“直接生成”

状态机：`待试采 → 试采成功 → 数据映射通过 → 真实数据渲染通过 → 生效`。

首采凭据绑定会话代次、SourceSpec 摘要、完整采样及候选 ID，由原生层产生并检查；模型返回 `verified=true` 或伪造一份 stdout 没有效力。初始凭据最多保留五分钟，重连即失效。激活前必需源样本若早于 `max(30秒, 2×采样间隔)`，按同一命令自动刷新并重跑本地校验。

AI 生成方案期间，原面板继续使用当前版本。激活时同时核对候选 ID、定义版本、数据版本及样本所属源；失效的候选、旧命令晚到的结果均丢弃。

只改颜色、布局和组件时可以复用当前有效数据与首采记录，不重置采集或历史。改变取数命令、目标或解析/映射时，先验证新链路再替换；刷新页面不能绕过验证。

实际需要周期采样后才成立的值（例如速率）可以在首采通过后显示“等待第二个有效样本”，不能编造一个速率来让预检通过。空值与失败夹具是额外测试，不能代替第一次真实 SSH 执行。

## 8. 实现顺序与验收

1. **Capture 与首采凭据**：改造现有 SSH 采集返回对象，两种通道共用；没有真实凭据时创建接口拒绝激活。
2. **本地数据执行器**：先实现标准解析器、筛选、汇总、单位转换和绑定校验，再增加 join 和跨采样算子。未实现算子不得出现在 AI 能力声明中。
3. **统一 Dataset 与历史**：Go 管理数据质量、版本和时间语义；前端接收已组装好的数据。
4. **组件接入与创建事务**：沿用编译和隔离运行环境，用真实 Dataset 预检；当前错误、旧成功数据、局部未知均可见。
5. **AI 交互**：先提交试采请求、读取真实结果，再提交数据计划，最后输出组件与布局。直接提交未经试采的完整面板只返回“需要首采”，不能自动生效。
6. **兼容与迁移**：保留 v1 已运行组件的读路径；旧命令/契约可以转换为单源 JSON、KV 或文本解析器。恢复历史定义重新试采，逐步让新生成使用 v2。

首版沿用已有单次输出 256 KiB、统一快照 64 KiB、历史 128 KiB / 600 点 / 10 分钟的上限；增加每计划最多 8 个源、32 个转换节点，RE2 匹配、最多 1000 行映射输出和跨源连接规模检查。本地执行器按上下文取消并限制中间结果，渲染继续使用现有 VM 和节点预算。

验收必须执行整条链路，而不只是编译组件：

- 在没有 Python/jq 的测试环境执行普通取数命令，本地映射出 Nginx 指标、表格和趋势。
- 首采工具缺失/权限错误/超时，界面保留具体采集诊断，不生成“正常”面板。
- 列格式变化、缺列、混入诊断行，本地报告解析节点与原因，不静默归零。
- 两个真实计数样本产生正确速率；重启、断线和计数重置形成缺口。
- 仅改变布局不改变命令、历史与采样节奏；换命令的候选失败时原面板继续运行。
- 伪造、跨连接、过期及命令不一致的首采凭据被拒绝；主会话通道也经过相同验证。

## 9. 已实现接口与兼容边界

`SourceSpec.target` 的归属由当前 App/SSH 连接持有，不能由 AI 覆盖；Capture 的 `sessionId` 是原生采集会话标识，与 AI 聊天会话无关。SSH 连接结束会使凭据失效。

实际 AI 接口沿用 `panelActions`，`panel.pipeline` 使用强类型 JSON 对象：

1. `action=probe`：`panel.id` 固定，`pipeline={protocolVersion:2,sources:[{id,command,timeoutMs,required}]}`。
2. `action=map`：`pipeline={protocolVersion:2,refs:[{sourceId,captureId}],dataPlan:{parsers,steps,outputs}}`。返回原生 `mapId` 和真实 Dataset；失败返回具体阶段、节点和原因。
3. `action=create/update`：`pipeline={protocolVersion:2,mapId,bindings}`，组件放在 `panel.view`，使用 `sdkVersion=2,stateVersion=1`。`command/collector/content` 为空，旧 `data` 契约为 null。间隔使用 `panel.intervalMs`，更新校验 `panel.expectedRevision`。

三阶段分别返回并等待原生回执；不能在同轮先后猜测 ID。Strict JSON Schema 中未使用的可选字段可以为 null。接口不接受 AI 自带 Capture 或执行成功标志。凭据最多保留五分钟；生效前过期样本按原命令自动刷新，并发放新的预检候选 ID。连续两次刷新后仍无法满足新鲜度的方案会失败，防止无限重采。

执行器实现文件为 `internal/monitorplan/`，连接和事务在 `internal/app/monitor_pipeline.go`。解析器支持 lines、kv、delimited、regex_rows、json。首版包含上文算子，另提供 `first(field)` 从恰好一行取出标量、`convert_type(to)`、`enum(names:[{from,to}])`、`rolling_avg(window)`。join 为显式 inner join；空表仍保持列结构。聚合不接受经过 limit 截短的输入。

`delta/rate/rolling_avg.other` 可引用稳定的服务实例 ID 或启动时间节点；实例变化会断开历史并重新建立基线。不提供实例标识时只能检测计数回退、采集失败和时间间隔中断，不能凭计数上升推断服务没有重启。首版不修正计数器回绕，不支持部分集合汇总；这类值保守返回 unknown。rolling_avg 的 window 单位为有效采样点数。独立来源按一次采集批次发布，同时保留各自的实际采样时间。

原生层在本次数据不可用时保留未知值、节点诊断和历史缺口；上次成功值单独标记 stale。等待主 SSH 提示符导致数据超时，也会显示过期状态。SDK v2 组件只接收绑定对象，不接收原始 Shell 内容。

已有 v1 面板可以继续运行和修改展示；改变其命令或旧数据契约需要迁移至 v2 首采流程。v1、v2 的历史恢复都先重新执行 SSH 命令，再验证数据、编译和真实数据渲染。新建 v1 自定义面板已拒绝。纯文本面板与独立告警保留原有接口。

仅改变布局保留命令、数据历史与采样时序；改变绑定名时会重建绑定后的历史，避免把旧别名曲线错误复用。保存定义包含 sources/dataPlan/bindings/view，不包含原始输出、Capture 或 mapId。

## 10. 验证与复现

Go 回归覆盖两种采集通道的原始流、退出状态、首采凭据、失败/过期/跨连接拒绝、数据映射与时间算子、自动 AI 阶段继续、原子替换和历史恢复：

```sh
go test ./internal/monitorplan ./internal/monitor ./internal/widgets ./internal/agent ./internal/app ./internal/desktop ./internal/policy ./internal/alerts
```

前端验证使用 Go 集成测试实际执行的 Shell 输出生成 fixture；模拟 SSH 可执行文件的 PATH 只有 sh，没有 Python/jq。该 fixture 明确是测试数据，不是生产服务器状态：

```sh
mkdir -p .cache
AISSH_PIPELINE_FIXTURE="$PWD/.cache/widget-v2-fixture.json" go test ./internal/app -run '^TestPipelineActualSSHFirstCaptureMapCompileAndActivateWithoutPython$' -count=1
go build -o .cache/widget-test-aissh ./cmd/aissh
(cd frontend && npm run build)
node --test scripts/widget-tests/runtime.test.mjs scripts/widget-tests/ui.test.mjs
go build -tags desktop,production -o .cache/widget-native-smoke ./scripts/widget-tests/native
.cache/widget-native-smoke
```

原生测试需 macOS 桌面会话；UI 测试需安装 Playwright 浏览器。测试不会读取用户 SSH 凭据或连接生产主机。单个 fixture 包含原生编译制品和绑定后的真实测试采样，可直接用于 Worker/WASM 测试。
