# 外联系统：使用与实现说明

2026-09-26。本文对应当前代码；完整领域设计见 [架构文档](external-systems-architecture.md)。

## 使用流程

1. 添加连接，选择所属系统、生产/测试/开发环境、凭据和网络路径。后台探索读取文档或元数据。
2. 打开「操作目录 → 接口目录」，审查候选的参数、映射、实际效果和风险，解决阻塞问题后启用。也可手工登记接口。
3. 在「权限管理」开放必要的连接能力，并给本机、SSH 主机或特定会话授予确切接口版本。删除需要同时有 `write` 和 `delete`；服务操作需要 `operate`，远端权限变更需要 `remote_admin`。
4. 在「风险规则」设置执行限制。在「预览与调用」输入参数，先看决策与准备好的请求，再确认本次调用。模型没有修改授权或批准请求的工具。
5. SSH 使用 `system.describe` 获取接口契约，用 `system.invoke` 提交接口 ID、修订版与参数。调用者身份来自创建 Executor 的本地受信上下文。

## 代码边界

| 模块 | 职责 |
| --- | --- |
| `internal/catalog` | 系统归属、连接环境/地址/路由、凭据引用；版本门禁 |
| `internal/externals` | 接口 Schema、协议请求准备、授权、风险、独立目录、审计和迁移 |
| `internal/resources` | 只读探索、OpenAPI 契约提取、统一调用网关、HTTP/SQL/Redis 执行 |
| `internal/desktop/externals.go` | 本地管理接口、后台探索任务、模拟判定、一次确认令牌 |
| `internal/app` | SSH 提案预览、确认后执行；保留命令不会落入远端 Shell |
| `frontend/src/ExternalConsole.tsx` | 接口、权限、风险、预览、审计五个管理页签 |

目录位于配置目录下 `external-systems/<connectionId>/`：`catalog.json` 为原子快照，包含当前接口、候选、最近 200 个旧定义、授权、风险及最近 200 次变更；`discovery.json` 为最近一次探索结果；`job.json` 为任务状态；`invocations.jsonl` 保存调用事件（超过 8 MiB 滚动到 `.1`，界面显示最近 200 条）。文件权限为 0600，目录为 0700。接口目录上限 8 MiB，独立于连接文件。

系统当前用连接上的 `systemId/systemName` 表达归属，没有另建系统数据库。同一系统、同类型连接可以复制接口为候选，分别审查和授权。当前和历史定义以不可变修订内容保留；操作生命周期可因远端文档变化停用，策略快照修订号同步变化。

## 判定与确认

`参数校验 → 固定模板准备 → 主体/接口版本/能力/参数范围授权 → 风险判定 → 精确请求确认 → 再校验 → 原子消费次数与记录发起 → 适配器 → 结果记录`。

一条授权必须完整覆盖请求；不拼接多条不完整授权。连接禁止规则和命中的拒绝授权优先。只允许已审查或内置接口；未知效果或风险拒绝执行。即使文档推断为只读，也不会直接获得调用授权。

授权支持 `values`（允许值）、`prefix`（字符串前缀）、`maximum`（数字上限），字段路径使用点号。`allowedFields` 是输入叶字段白名单；读取结果的字段限制需要体现在 SQL 投影或外部 API 中。会话授权以 `system.describe` 返回的 `sessionId` 为主体，进程更换后失效。有效期以本地时钟检查。一次授权可设置 `maxUses=1` 并绑定预览的 `requestDigest`。

确认保存于当前执行进程内存，绑定连接及凭据快照、调用者、参数、接口内容、策略版本，五分钟失效且只消费一次。每次执行从磁盘重读规则；次数通过跨进程文件锁消费，失败或结果不确定的尝试也计次。重启不会恢复未完成的确认。

写操作至少为中风险，删除/服务操作/权限操作至少为高风险，均需逐次确认。敏感、删除和严重风险默认禁止，高风险默认不开启。网络错误导致写入是否成功无法证明时返回 `outcome_unknown`，不自动重试；请先通过独立只读接口核对。审计只有 `started` 没有结束事件，也表示结果尚不能确认。

## 接口模板示例

HTTP 更新：

```json
{
  "inputSchema": {
    "type": "object",
    "properties": {
      "id": {"type":"string","pattern":"^[a-zA-Z0-9_-]+$"},
      "body": {"type":"object","properties":{"owner":{"type":"string","maxLength":100}},"required":["owner"],"additionalProperties":false}
    },
    "required": ["id","body"],
    "additionalProperties": false
  },
  "request": {
    "method": "PATCH",
    "path": "/assets/{id}",
    "parameters": [{"input":"id","in":"path","name":"id"}],
    "bodyInput": "body"
  }
}
```

PostgreSQL 固定模板 `UPDATE assets SET owner = $1 WHERE id = $2` 配置 `sqlParameters: ["owner", "id"]`，值由驱动绑定。只读接口在只读事务内执行，未支持的函数、锁和副作用语义会被拒绝；此保守语法检查不替代数据库账号权限与服务端约束。写入事务在影响行数超过风险上限时回滚；限制直接影响的行数，不保证统计触发器或外部函数副作用。MySQL 目前只开放读取，尚未证明存储引擎及触发器的事务回滚边界，写入明确拒绝。

Redis 固定 `redisCommand: "SET"`、`redisParameters: ["key", "value"]`，配合 key 的前缀授权限定对象。写适配器仅允许单键 SET、EXPIRE、PERSIST、DEL、UNLINK；EXPIRE 必须为正整数秒。FLUSH、CONFIG、SHUTDOWN、脚本及模块命令不开放。

## Schema 与探索支持范围

输入支持 object/array/string/integer/number/boolean/null、properties、required、enum、数值/长度/数组数量边界、pattern、items；对象拒绝额外字段。未知关键字、组合 Schema、外部引用、未解析 `$ref` 不会被忽略。HTTP 输出可选用相同子集校验完整 JSON；其他适配器暂不支持输出 Schema。

OpenAPI JSON 目前提取路径/查询参数的 schema 及 application/json 请求体，参数名在输入中使用 `path_<name>` 或 `query_<name>`。路径参数只允许单个路径段；查询仅支持标量。缺失类型、header/cookie 参数、服务器覆盖、未解析引用进入阻塞候选。Swagger 2 参数需手工补齐 Schema。响应 Schema 可手工登记，尚未从 responses 自动展开。

探索总预算 25 秒，每次探测 5 秒，最多提取 100 项接口，不执行文档中的业务操作。报告不覆盖授权。远端契约变化会把对应已审查接口转回草稿；完整扫描里消失的探索接口标记废弃。失败/部分扫描保留既有定义。任务支持取消，应用重启后显示中断；当前保留最近一次任务和报告，没有历史任务队列或自动周期重扫。

## 迁移

首次访问/管理旧连接时，先落盘独立目录，再备份 `connections.v1.backup.json` 并将连接配置升级为 v2。旧客户端拒绝 v2，避免按旧授权执行。原读取范围只迁移到固定内置读取接口；HTTP 业务操作成为待审查候选。已启用连接的目录丢失时直接拒绝调用，不重置权限。修改目标地址、数据库、SSH 路由目标或 TLS 设置后，接口必须重新确认。

## 验证

Go 测试覆盖主体与参数约束、显式拒绝、到期/撤销、旧接口版本、权限与风险独立、Schema 越界、原子单次授权、迁移丢失时拒绝、确认失效/重放、旧入口拒绝、认证脱敏、重定向、未知写结果与探索差异。`scripts/test-resource-discovery.mjs` 用模拟 Wails 桥验证审查、授权、风险保存、预览、确认、撤销及审计界面；不访问真实外联系统。
