package externals

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// This deliberately closed subset fails on unsupported constraints instead of
// silently relaxing a imported contract. All object inputs reject extra keys.
func CheckSchema(s Schema) error { return checkSchema(s, 0) }
func checkSchema(s Schema, depth int) error {
	if depth > 12 {
		return errors.New("Schema 嵌套超过 12 层")
	}
	for k := range s {
		if !slices.Contains([]string{"$schema", "title", "description", "type", "properties", "required", "additionalProperties", "items", "enum", "minimum", "maximum", "minLength", "maxLength", "pattern", "minItems", "maxItems"}, k) {
			return fmt.Errorf("暂不支持 Schema 约束 %s", k)
		}
	}
	if dialect, ok := s["$schema"]; ok && dialect != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("不支持的 Schema dialect")
	}
	typ, _ := s["type"].(string)
	if !slices.Contains([]string{"object", "array", "string", "integer", "number", "boolean", "null"}, typ) {
		return errors.New("Schema 必须有明确的单一 type")
	}
	for _, key := range []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems"} {
		if v, ok := s[key]; ok {
			n, ok := number(v)
			if !ok || math.IsInf(n, 0) || math.IsNaN(n) {
				return fmt.Errorf("%s 必须是有限数字", key)
			}
			if strings.Contains(key, "Length") || strings.Contains(key, "Items") {
				if n < 0 || math.Trunc(n) != n {
					return fmt.Errorf("%s 必须为非负整数", key)
				}
			}
		}
	}
	for _, pair := range [][2]string{{"minimum", "maximum"}, {"minLength", "maxLength"}, {"minItems", "maxItems"}} {
		lo, lok := number(s[pair[0]])
		hi, hok := number(s[pair[1]])
		if lok && hok && lo > hi {
			return errors.New("Schema 最小值大于最大值")
		}
	}
	if pattern, ok := s["pattern"]; ok {
		p, ok := pattern.(string)
		if !ok || len(p) > 1024 {
			return errors.New("pattern 无效")
		}
		if _, err := regexp.Compile(p); err != nil {
			return errors.New("不支持的正则表达式")
		}
	}
	if v, ok := s["enum"]; ok {
		a, ok := v.([]any)
		if !ok || len(a) == 0 || len(a) > 200 {
			return errors.New("enum 必须为非空数组且最多 200 项")
		}
	}
	if typ == "object" {
		props, ok := object(s["properties"])
		if !ok && s["properties"] != nil {
			return errors.New("properties 必须是对象")
		}
		if len(props) > 100 {
			return errors.New("对象最多 100 个字段")
		}
		if extra, exists := s["additionalProperties"]; exists && extra != false {
			return errors.New("只支持 additionalProperties=false")
		}
		for name, v := range props {
			child, ok := object(v)
			if !ok || name == "" || len(name) > 200 || strings.ContainsAny(name, ".[]\x00") {
				return errors.New("字段 Schema 无效")
			}
			if err := checkSchema(child, depth+1); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if v, ok := s["required"]; ok {
			required, ok := stringsArray(v)
			if !ok {
				return errors.New("required 必须是字符串数组")
			}
			for _, key := range required {
				if _, ok := props[key]; !ok {
					return errors.New("required 引用了未定义字段")
				}
			}
		}
	}
	if typ == "array" {
		child, ok := object(s["items"])
		if !ok {
			return errors.New("数组需要 items Schema")
		}
		return checkSchema(child, depth+1)
	}
	return nil
}
func object(v any) (map[string]any, bool) {
	switch v := v.(type) {
	case map[string]any:
		return v, true
	case Schema:
		return v, true
	}
	return nil, false
}
func number(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, e := v.Float64()
		return n, e == nil
	}
	return 0, false
}
func stringsArray(v any) ([]string, bool) {
	if a, ok := v.([]string); ok {
		return a, true
	}
	a, ok := v.([]any)
	if !ok {
		return nil, false
	}
	r := []string{}
	for _, v := range a {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		r = append(r, s)
	}
	return r, true
}
func ValidateArguments(s Schema, args map[string]any) error {
	if err := CheckSchema(s); err != nil {
		return err
	}
	return validateValue(s, args, "arguments", 0)
}
func validateValue(s Schema, v any, path string, depth int) error {
	if depth > 12 {
		return errors.New("输入嵌套过深")
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		for _, item := range values {
			if equal(item, v) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%s 不在允许值中", path)
		}
	}
	switch s["type"] {
	case "object":
		o, ok := object(v)
		if !ok {
			return fmt.Errorf("%s 必须为对象", path)
		}
		props, _ := object(s["properties"])
		required, _ := stringsArray(s["required"])
		for _, k := range required {
			if _, ok := o[k]; !ok {
				return fmt.Errorf("%s.%s 为必填参数", path, k)
			}
		}
		for k, x := range o {
			p, ok := object(props[k])
			if !ok {
				return fmt.Errorf("%s.%s 未在接口中定义", path, k)
			}
			if err := validateValue(p, x, path+"."+k, depth+1); err != nil {
				return err
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%s 必须为数组", path)
		}
		if len(a) > 1000 {
			return errors.New("数组最多 1000 项")
		}
		if err := bounds(s, float64(len(a)), "minItems", "maxItems", path); err != nil {
			return err
		}
		child, _ := object(s["items"])
		for i, x := range a {
			if err := validateValue(child, x, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
				return err
			}
		}
	case "string":
		value, ok := v.(string)
		if !ok || !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > 32768 {
			return fmt.Errorf("%s 必须为不超过 32 KB 的有效字符串", path)
		}
		if err := bounds(s, float64(utf8.RuneCountInString(value)), "minLength", "maxLength", path); err != nil {
			return err
		}
		if p, ok := s["pattern"].(string); ok {
			if !regexp.MustCompile(p).MatchString(value) {
				return fmt.Errorf("%s 不符合格式约束", path)
			}
		}
	case "integer", "number":
		n, ok := number(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || (s["type"] == "integer" && math.Trunc(n) != n) {
			return fmt.Errorf("%s 数字类型无效", path)
		}
		if s["type"] == "integer" && math.Abs(n) > 9007199254740991 {
			return fmt.Errorf("%s 超出安全整数范围，请用字符串表示大整数标识", path)
		}
		return bounds(s, n, "minimum", "maximum", path)
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("%s 必须为布尔值", path)
		}
	case "null":
		if v != nil {
			return fmt.Errorf("%s 必须为空", path)
		}
	}
	return nil
}
func bounds(s map[string]any, n float64, lo, hi, path string) error {
	if v, ok := number(s[lo]); ok && n < v {
		return fmt.Errorf("%s 小于下限", path)
	}
	if v, ok := number(s[hi]); ok && n > v {
		return fmt.Errorf("%s 超过上限", path)
	}
	return nil
}
func equal(a, b any) bool { return reflect.DeepEqual(a, b) || Digest(a) == Digest(b) }
func EmptyInput() Schema {
	return Schema{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
}

func ValidateOutput(s Schema, value any) error {
	if err := CheckSchema(s); err != nil {
		return err
	}
	return validateValue(s, value, "response", 0)
}
