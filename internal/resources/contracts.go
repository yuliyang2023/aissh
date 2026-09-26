package resources

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
)

func contract(cap catalog.ResourceCapability, api apiOperation, common []apiParameter, override bool) externals.Definition {
	d := externals.LegacyCandidates(catalog.ResourceDiscovery{Operations: []catalog.ResourceCapability{cap}}, "http")[0]
	d.Blocked = ""
	d.Description = clip(api.Description, 4000)
	properties := map[string]any{}
	required := []string{}
	byKey := map[string]apiParameter{}
	order := []string{}
	for _, p := range append(common, api.Parameters...) {
		key := p.In + ":" + p.Name
		if _, ok := byKey[key]; !ok {
			order = append(order, key)
		}
		byKey[key] = p
	}
	for _, key := range order {
		p := byKey[key]
		if p.Ref != "" || p.Name == "" || (p.In != "path" && p.In != "query") {
			d.Blocked = "存在未解析引用或不支持的参数位置"
			continue
		}
		name := p.In + "_" + p.Name
		schema := p.Schema
		if len(schema) == 0 && p.Type != "" {
			schema = externals.Schema{"type": p.Type}
			d.Blocked = "Swagger 2 参数需要补全 Schema 约束后审查"
		}
		if len(schema) == 0 {
			d.Blocked = "参数缺少类型定义：" + p.Name
			schema = externals.Schema{"type": "string"}
		}
		if err := externals.CheckSchema(schema); err != nil {
			d.Blocked = fmt.Sprintf("参数 %s：%s", p.Name, err)
		}
		properties[name] = schema
		d.Request.Parameters = append(d.Request.Parameters, externals.Parameter{Input: name, In: p.In, Name: p.Name})
		if p.Required || p.In == "path" {
			required = append(required, name)
		}
	}
	if len(api.RequestBody) > 0 {
		var body struct {
			Ref      string `json:"$ref"`
			Required bool   `json:"required"`
			Content  map[string]struct {
				Schema externals.Schema `json:"schema"`
			} `json:"content"`
		}
		if json.Unmarshal(api.RequestBody, &body) != nil || body.Ref != "" || len(body.Content["application/json"].Schema) == 0 {
			d.Blocked = "请求体需要完整的 application/json Schema"
		} else {
			schema := body.Content["application/json"].Schema
			if err := externals.CheckSchema(schema); err != nil {
				d.Blocked = "请求体：" + err.Error()
			}
			properties["body"] = schema
			d.Request.BodyInput = "body"
			if body.Required {
				required = append(required, "body")
			}
		}
	}
	d.InputSchema = externals.Schema{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	if override {
		d.Blocked = "接口覆盖了服务器地址，请核对并明确此连接内的路径"
	}
	if cap.Risk == "dangerous" {
		text := strings.ToLower(cap.Path + " " + api.ID + " " + api.Summary)
		d.Assessment.Effects.Destructive = cap.Method == "DELETE" || strings.Contains(text, "delete") || strings.Contains(text, "删除") || strings.Contains(text, "flush") || strings.Contains(text, "truncate")
		d.Assessment.Effects.PermissionChange = strings.Contains(text, "permission") || strings.Contains(text, "grant") || strings.Contains(text, "权限")
		d.Assessment.Effects.ServiceImpact = !d.Assessment.Effects.Destructive && !d.Assessment.Effects.PermissionChange
	}
	return d
}
