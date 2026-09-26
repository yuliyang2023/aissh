package resources

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
)

var dangerousWords = regexp.MustCompile(`(?i)(\b(delete|remove|drop|truncate|purge|flush|reset|restart|reboot|shutdown|destroy|revoke|grant|permission|deploy|execute|shell)\b|删除|清空|重启|销毁|权限|发布|执行脚本)`)
var writeWords = regexp.MustCompile(`(?i)(\b(create|update|insert|edit|save|set|enable|disable|start|stop|send|submit|import|upload|trigger|sync|rotate)\b|新增|修改|保存|启用|停用|发送|提交|导入|触发|同步)`)
var readWords = regexp.MustCompile(`(?i)(\b(get|list|read|query|search|fetch|health|status|metrics)\b|查询|获取|列表|读取|检索|状态|健康)`)

// HTTP verbs cannot prove read-only semantics. Classifications are advisory
// candidate assessments; they never grant permission to execute.
var camelWords = regexp.MustCompile(`([a-z])([A-Z])`)

func classifyHTTP(method, description string) (string, string) {
	description = strings.ReplaceAll(camelWords.ReplaceAllString(description, "$1 $2"), "_", " ")
	if method == "DELETE" || dangerousWords.MatchString(description) {
		return "dangerous", "方法或文档包含删除、执行、权限或服务中断语义"
	}
	if method == "POST" || method == "PUT" || method == "PATCH" || writeWords.MatchString(description) {
		return "write", "方法或文档包含创建、修改或触发动作语义"
	}
	if (method == "GET" || method == "HEAD") && readWords.MatchString(description) {
		return "read", "文档描述为查询/读取；仅为文档分类，未试执行业务操作"
	}
	return "unknown", "缺少足够的只读语义依据，需要确认实际副作用"
}

type apiParameter struct {
	Schema   externals.Schema `json:"schema"`
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	In       string           `json:"in"`
	Required bool             `json:"required"`
	Ref      string           `json:"$ref"`
}
type apiOperation struct {
	ID          string          `json:"operationId"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Parameters  []apiParameter  `json:"parameters"`
	RequestBody json.RawMessage `json:"requestBody"`
	Servers     json.RawMessage `json:"servers"`
}

func discoverOpenAPI(r catalog.Resource, source, body string) ([]catalog.ResourceCapability, []string, error) {
	var doc struct {
		OpenAPI  string `json:"openapi"`
		Swagger  string `json:"swagger"`
		BasePath string `json:"basePath"`
		Servers  []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if json.Unmarshal([]byte(body), &doc) != nil || (doc.OpenAPI == "" && doc.Swagger != "2.0") || len(doc.Paths) == 0 {
		return nil, nil, errors.New("未发现有效的 OpenAPI / Swagger JSON 路径定义")
	}
	notes := []string{}
	base, _ := catalog.ServiceURL(r.Endpoint)
	prefix := ""
	server := doc.BasePath
	if len(doc.Servers) > 0 {
		server = doc.Servers[0].URL
	}
	if server != "" {
		u, err := url.Parse(server)
		if err != nil || strings.ContainsAny(server, "{}") {
			return nil, nil, errors.New("文档服务器地址含未解析变量，请配置对应的 API 基础地址")
		}
		// Relative server URLs resolve against the document's URL, as specified
		// by OpenAPI. Never follow a discovered host or escape the saved base.
		docURL, err := requestURL(r.Endpoint, source)
		if err != nil {
			return nil, nil, err
		}
		u = docURL.ResolveReference(u)
		basePath := strings.TrimRight(base.Path, "/")
		if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != basePath && !strings.HasPrefix(u.Path, basePath+"/")) {
			return nil, nil, errors.New("文档的 API 地址不在此连接范围内，请将服务地址设为对应 API 基础地址")
		}
		prefix = strings.TrimRight(strings.TrimPrefix(u.Path, basePath), "/")
	}
	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	ops := []catalog.ResourceCapability{}
	for _, path := range paths {
		item := doc.Paths[path]
		var common []apiParameter
		_ = json.Unmarshal(item["parameters"], &common)
		if len(item["$ref"]) > 0 {
			notes = append(notes, "路径引用未展开："+clip(path, 100))
			continue
		}
		for _, method := range []string{"get", "head", "post", "put", "patch", "delete", "options", "trace"} {
			raw, ok := item[method]
			if !ok {
				continue
			}
			if len(ops) == 100 {
				notes = append(notes, "仅整理前 100 项接口，其余操作仍待确认。")
				return ops, notes, nil
			}
			var op apiOperation
			if json.Unmarshal(raw, &op) != nil {
				notes = append(notes, "接口定义无法解析："+clip(path, 100))
				continue
			}
			fullPath := prefix + path
			if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
				notes = append(notes, "忽略无效路径："+clip(path, 100))
				continue
			}
			if _, err := requestURL(r.Endpoint, fullPath); err != nil {
				notes = append(notes, "忽略超出范围的路径："+clip(path, 100))
				continue
			}
			method := strings.ToUpper(method)
			risk, reason := classifyHTTP(method, path+" "+op.ID+" "+op.Summary+" "+op.Description)
			parameters := []string{}
			for _, p := range append(common, op.Parameters...) {
				if p.Ref != "" {
					parameters = append(parameters, "引用参数（待补全）")
					if risk == "read" {
						risk = "unknown"
					}
					continue
				}
				label := p.In + ":" + p.Name
				if p.Required {
					label += "（必填）"
				}
				parameters = append(parameters, label)
			}
			if len(op.RequestBody) > 0 {
				parameters = append(parameters, "body：结构见接口文档")
			}
			if len(op.Servers) > 0 || len(item["servers"]) > 0 {
				if risk == "read" {
					risk = "unknown"
				}
				parameters = append(parameters, "接口覆盖了服务器地址，需要核对调用范围")
			}
			if risk == "unknown" {
				reason = "语义、参数或调用地址不完整，需要确认实际副作用"
			}
			name := op.Summary
			if name == "" {
				name = op.ID
			}
			if name == "" {
				name = method + " " + path
			}
			capability := catalog.ResourceCapability{Name: clip(name, 120), Risk: risk, Method: method, Path: fullPath, Parameters: clip(strings.Join(parameters, "；"), 600), Evidence: source + " · " + reason}

			definition := contract(capability, op, common, len(op.Servers) > 0 || len(item["servers"]) > 0)
			capability.Contract, _ = json.Marshal(definition)
			capability.Example = Operation{ResourceID: r.ID, Action: "system.invoke", OperationID: definition.ID, OperationRevision: 1, Arguments: map[string]any{}}.Command()
			ops = append(ops, capability)
		}
	}
	if len(ops) == 0 {
		return nil, nil, errors.New("文档没有可整理的接口操作")
	}
	if len(notes) > 20 {
		notes = notes[:20]
	}
	return ops, notes, nil
}

var sqlMutations = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|truncate|alter|grant|revoke|create|replace|call|copy|load|into|lock)\b`)
var sqlCalls = regexp.MustCompile(`(?i)\b([a-z_][a-z_0-9]*)\s*\(`)

func sqlRisk(query string) string {
	// Complex SQL remains reviewable, but never inherits a read-only grant.
	upper := strings.ToUpper(strings.TrimSpace(query))
	if sqlMutations.MatchString(query) {
		return "dangerous"
	}
	if !strings.HasPrefix(upper, "SELECT ") || strings.ContainsAny(query, ";#") || strings.Contains(query, "--") || strings.Contains(query, "/*") {
		return "unknown"
	}
	for _, call := range sqlCalls.FindAllStringSubmatch(query, -1) {
		switch strings.ToUpper(call[1]) {
		case "COUNT", "SUM", "MIN", "MAX", "AVG", "DATABASE", "IN":
		default:
			return "unknown"
		}
	}
	return "read"
}
