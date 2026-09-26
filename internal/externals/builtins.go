package externals

import (
	"encoding/json"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"strings"
)

func Builtins(kind string) []Definition {
	ops := []Definition{}
	add := func(name string, input Schema, req Request) {
		ops = append(ops, Definition{ID: OperationID(kind, name), Revision: 1, Name: name, Kind: kind, InputSchema: input, Request: req, Assessment: Assessment{Effects: Effects{Access: "read"}, Risk: "low", Assurance: "builtin", Evidence: "固定只读适配器"}, Lifecycle: "active", Source: "builtin", UpdatedAt: Now()})
	}
	switch kind {
	case "redis":
		for _, cmd := range []string{"PING", "INFO", "DBSIZE", "GET", "TTL", "PTTL", "TYPE", "STRLEN", "SCARD", "LLEN", "HLEN", "ZCARD"} {
			input := EmptyInput()
			req := Request{RedisCommand: cmd, Parameters: []Parameter{}}
			if cmd != "PING" && cmd != "INFO" && cmd != "DBSIZE" {
				input["properties"] = map[string]any{"key": Schema{"type": "string", "minLength": 1, "maxLength": 4096}}
				input["required"] = []string{"key"}
				req.RedisParameters = []string{"key"}
			}
			add(cmd, input, req)
		}
	case "postgres", "mysql":
		add("连接检查", EmptyInput(), Request{Query: "SELECT 1", Parameters: []Parameter{}})
		add("查看表目录", EmptyInput(), Request{Query: "SELECT table_schema, table_name, table_type FROM information_schema.tables ORDER BY table_schema, table_name LIMIT 100", Parameters: []Parameter{}})
	}
	return ops
}

func LegacyCandidates(report catalog.ResourceDiscovery, kind string) []Definition {
	result := []Definition{}
	for _, cap := range report.Operations {
		if len(cap.Contract) > 0 {
			var op Definition
			if json.Unmarshal(cap.Contract, &op) == nil && op.Kind == kind {
				result = append(result, op)
				continue
			}
		}
		if kind != "http" || cap.Method == "" {
			continue
		}
		access, risk := "unknown", "unknown"
		effect := Effects{Access: access}
		switch cap.Risk {
		case "read":
			access, risk = "read", "low"
		case "write":
			access, risk = "write", "medium"
		case "dangerous":
			access, risk = "write", "high"
			effect.Destructive = true
		}
		effect.Access = access
		blocked := ""
		if cap.Parameters != "" || strings.Contains(cap.Path, "{") {
			blocked = "旧探索没有结构化参数，请补齐输入 Schema 和参数映射"
		}
		result = append(result, Definition{ID: OperationID(kind, cap.Method+" "+cap.Path), Revision: 1, Name: cap.Name, Kind: kind, InputSchema: EmptyInput(), Request: Request{Method: cap.Method, Path: cap.Path, Parameters: []Parameter{}}, Assessment: Assessment{Effects: effect, Risk: risk, Assurance: "inferred", Evidence: cap.Evidence}, Lifecycle: "draft", Source: "discovery", Blocked: blocked, UpdatedAt: Now()})
	}
	return result
}
