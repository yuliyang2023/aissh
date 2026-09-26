package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
)

// Explore is a trusted onboarding action. It only runs the fixed metadata
// probes below, never a command supplied by a model or discovered document.
// It does not create an Executor credential grant or change saved permissions.
func Explore(ctx context.Context, store *catalog.Store, id, helper string) (catalog.ResourceDiscovery, error) {
	d, err := store.Load()
	if err != nil {
		return catalog.ResourceDiscovery{}, err
	}
	r, err := d.Resource(id)
	if err != nil {
		return catalog.ResourceDiscovery{}, err
	}
	c, _ := d.Credential(r.CredentialID)
	e := New(store, r.HostID, helper, nil)
	report := catalog.ResourceDiscovery{
		Status: "failed", Binding: catalog.ResourceBinding(r, d), ExploredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Notes: []string{}, Operations: []catalog.ResourceCapability{},
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	probe := func(op Operation) (string, error) {
		child, done := context.WithTimeout(ctx, 5*time.Second)
		defer done()
		op.ResourceID = id
		return e.executeBound(child, d, r, c, op)
	}
	switch r.Kind {
	case "http":
		paths := []string{"/openapi.json", "/swagger.json", "/v3/api-docs", "/api-docs"}
		if r.DiscoveryPath != "" {
			paths = []string{r.DiscoveryPath}
		}
		for _, path := range paths {
			if u, err := requestURL(r.Endpoint, path); err != nil || u.RawQuery != "" {
				report.Notes = append(report.Notes, "接口文档路径必须在当前系统范围内，且不能包含查询参数。")
				continue
			} else if risk, _ := classifyHTTP("GET", u.Path); risk == "write" || risk == "dangerous" {
				report.Notes = append(report.Notes, "接口文档路径包含写入或危险动作，未请求："+path)
				continue
			}
			output, err := probe(Operation{Action: "http.get", Path: path})
			if err != nil {
				report.Notes = append(report.Notes, path+"："+err.Error())
				continue
			}
			status, body, _ := strings.Cut(output, "\n")
			if !strings.HasPrefix(status, "HTTP 2") {
				report.Notes = append(report.Notes, path+"："+status)
				continue
			}
			ops, notes, err := discoverOpenAPI(r, path, body)
			if err != nil {
				report.Notes = append(report.Notes, path+"："+err.Error())
				continue
			}
			report.Operations = ops
			report.Notes = append(report.Notes, notes...)
			report.Status = "ready"
			if len(notes) > 0 {
				report.Status = "partial"
			}
			report.Summary = fmt.Sprintf("已读取 %s，整理 %d 项接口操作；分类来自文档，未试执行业务接口。", path, len(ops))
			break
		}
		if report.Status == "failed" {
			report.Status = "partial"
			report.Summary = "未找到可解析的 OpenAPI / Swagger JSON 文档，操作风险仍待确认。请补充接口文档路径或凭据后重新探索。"
		}
	case "postgres", "mysql":
		query := "SELECT table_schema, table_name, column_name, data_type FROM information_schema.columns WHERE table_schema NOT IN ('information_schema', 'pg_catalog') ORDER BY table_schema, table_name, ordinal_position LIMIT 100"
		if r.Kind == "mysql" {
			query = "SELECT table_schema, table_name, column_name, data_type FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position LIMIT 100"
		}
		output, err := probe(Operation{Action: "database.query", Query: query})
		if err != nil {
			report.Summary = "数据库元数据探索失败；请检查地址、凭据和访问路径。"
			report.Notes = append(report.Notes, err.Error())
			break
		}
		report.Status, report.Summary = "ready", "已在只读事务中读取表和字段元数据（最多 100 个字段），未读取业务记录。SQL 分类是类型规则；业务调用需保存固定参数化接口。"
		report.Operations = []catalog.ResourceCapability{
			{Name: "查询表和字段", Risk: "read", Evidence: "information_schema 元数据查询成功", Example: Operation{ResourceID: id, Action: "system.invoke", OperationID: externals.OperationID(r.Kind, "查看表目录"), OperationRevision: 1, Arguments: map[string]any{}}.Command()},
			{Name: "SELECT 普通数据查询", Risk: "read", Parameters: "明确表、字段、过滤条件与 LIMIT；函数调用、锁和外部副作用需单独审查", Evidence: "SQL 类型规则；通过只读事务执行"},
			{Name: "INSERT / UPDATE", Risk: "write", Evidence: "SQL 类型规则：新增或修改业务数据；需登记固定参数化接口、审查并授权"},
			{Name: "DELETE / DROP / TRUNCATE / ALTER / GRANT / REVOKE", Risk: "dangerous", Evidence: "SQL 类型规则：删除需单独授权；DDL 和远端权限语句不开放"},
			{Name: "存储过程、用户函数与其他语句", Risk: "unknown", Evidence: "仅凭查询语法不能确定副作用，需要逐项确认"},
		}
		var result struct {
			Rows      [][]any `json:"rows"`
			Truncated bool    `json:"truncated"`
		}
		if json.Unmarshal([]byte(output), &result) == nil {
			for _, row := range result.Rows {
				if len(row) == 4 {
					report.Notes = append(report.Notes, fmt.Sprintf("%v.%v · %v (%v)", row[0], row[1], row[2], row[3]))
				}
			}
			if len(result.Rows) >= 100 || result.Truncated {
				report.Status = "partial"
				report.Notes = append(report.Notes, "仅展示前 100 个字段，元数据可能不完整。")
			}
		}
	case "redis":
		output, err := probe(Operation{Action: "redis.read", Args: []string{"INFO"}})
		if err != nil {
			report.Summary = "Redis 元数据探索失败；请检查地址、凭据与 INFO 权限。"
			report.Notes = append(report.Notes, err.Error())
			break
		}
		report.Status, report.Summary = "ready", "已读取 Redis INFO；操作分类来自适配器规则，未读取业务键值，未验证写入权限。"
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "redis_version:") || strings.HasPrefix(line, "redis_mode:") {
				report.Notes = append(report.Notes, strings.TrimSpace(line))
			}
		}
		report.Operations = []catalog.ResourceCapability{
			{Name: "PING / INFO / DBSIZE", Risk: "read", Evidence: "适配器只读白名单；INFO 探测成功", Example: Operation{ResourceID: id, Action: "system.invoke", OperationID: externals.OperationID("redis", "INFO"), OperationRevision: 1, Arguments: map[string]any{}}.Command()},
			{Name: "GET / TTL / PTTL / TYPE / STRLEN / SCARD / LLEN / HLEN / ZCARD", Risk: "read", Parameters: "单个 key", Evidence: "适配器只读白名单"},
			{Name: "SET / HSET / LPUSH / EXPIRE", Risk: "write", Evidence: "Redis 类型规则：修改键值或有效期；仅支持已登记的单键 SET / EXPIRE / PERSIST"},
			{Name: "DEL / UNLINK / FLUSHDB / FLUSHALL / CONFIG SET / SHUTDOWN", Risk: "dangerous", Evidence: "Redis 类型规则：危险操作；仅支持已登记的单键 DEL / UNLINK，清库和服务配置禁止执行"},
			{Name: "EVAL / FCALL / 模块命令", Risk: "unknown", Evidence: "副作用取决于脚本、函数或模块；当前适配器不支持"},
		}
	}
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	// Store only bounded, redacted metadata, never raw response bodies or values.
	for i := range report.Notes {
		report.Notes[i] = clip(e.Filter.Text(report.Notes[i]), 512)
	}
	for i := range report.Operations {
		op := &report.Operations[i]
		if len(op.Contract) > 0 {
			var def externals.Definition
			if json.Unmarshal(op.Contract, &def) == nil {
				def.Name = e.Filter.Text(def.Name)
				def.Description = e.Filter.Text(def.Description)
				def.Assessment.Evidence = e.Filter.Text(def.Assessment.Evidence)
				raw, _ := json.Marshal(def)
				if e.Filter.Text(string(raw)) != string(raw) {
					op.Contract = nil
					op.Risk = "unknown"
					op.Evidence = "定义含敏感信息，未导入"
				}
			}
		}
		op.Name, op.Path = e.Filter.Text(op.Name), e.Filter.Text(op.Path)
		op.Parameters, op.Evidence = e.Filter.Text(op.Parameters), e.Filter.Text(op.Evidence)
		if op.Example != "" && op.Method == "GET" && len(op.Contract) == 0 {
			op.Example = ""
		}
	}
	if r.Managed {
		err = externals.NewStore(store).Import(id, report, externals.LegacyCandidates(report, r.Kind))
	} else {
		err = store.SaveResourceDiscovery(id, report)
	}
	if err != nil {
		return report, err
	}
	return report, nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
