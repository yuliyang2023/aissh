package agent

import (
	"encoding/json"
	"git.example.com/ai/wrd-ai-cli/internal/monitorplan"
	"reflect"
	"strings"
)

// Keep the strict tool schema in sync with the bounded native plan vocabulary.
func withPipelineSchema(base string) string {
	var root map[string]any
	if err := json.Unmarshal([]byte(base), &root); err != nil {
		panic(err)
	}
	props := root["properties"].(map[string]any)["panelActions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	props["action"].(map[string]any)["enum"] = []string{"open", "create", "update", "append", "close", "probe", "map"}
	panel := props["panel"].(map[string]any)
	panel["properties"].(map[string]any)["pipeline"] = strictPlanType(reflect.TypeOf((*monitorplan.Request)(nil)))
	panel["required"] = append(panel["required"].([]any), "pipeline")
	data, err := json.Marshal(root)
	if err != nil {
		panic(err)
	}
	return string(data)
}
func strictPlanType(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Pointer:
		return map[string]any{"anyOf": []any{map[string]any{"type": "null"}, strictPlanType(t.Elem())}}
	case reflect.Struct:
		p := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")
			key := tag[0]
			if key == "" || key == "-" {
				continue
			}
			schema := strictPlanType(f.Type)
			if strings.Contains(f.Tag.Get("json"), "omitempty") && f.Type.Kind() != reflect.Pointer {
				schema = map[string]any{"anyOf": []any{map[string]any{"type": "null"}, schema}}
			}
			p[key] = schema
			required = append(required, key)
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": p, "required": required}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": strictPlanType(t.Elem())}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	default:
		return map[string]any{"type": "integer"}
	}
}
