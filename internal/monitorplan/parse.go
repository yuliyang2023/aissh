package monitorplan

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

func convert(v any, typ string) (any, error) {
	if v == nil {
		return nil, fmt.Errorf("缺少字段")
	}
	switch typ {
	case "string":
		switch n := v.(type) {
		case string:
			return n, nil
		case json.Number:
			return string(n), nil
		case float64:
			return strconv.FormatFloat(n, 'g', -1, 64), nil
		case bool:
			return strconv.FormatBool(n), nil
		}
	case "number":
		var n float64
		var err error
		switch x := v.(type) {
		case string:
			n, err = strconv.ParseFloat(x, 64)
		case json.Number:
			n, err = x.Float64()
		case float64:
			n = x
		default:
			err = fmt.Errorf("不是数值")
		}
		if err == nil && finite(n) {
			return n, nil
		}
	case "boolean":
		switch n := v.(type) {
		case bool:
			return n, nil
		case string:
			if n == "true" {
				return true, nil
			}
			if n == "false" {
				return false, nil
			}
		}
	}
	return nil, fmt.Errorf("无法将 %v 解释为 %s", v, typ)
}
func finite(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 9007199254740991
}
func parse(p Parser, text string) ([]map[string]any, error) {
	if len(text) > MaxRaw {
		return nil, fmt.Errorf("原始输出超限")
	}
	rows := []map[string]any{}
	appendRow := func(raw map[string]any) error {
		if len(rows) >= MaxRows {
			return fmt.Errorf("解析超过 1000 行，请缩小采集范围")
		}
		r := map[string]any{}
		for _, c := range p.Columns {
			v, e := convert(raw[c.ID], c.Type)
			if e != nil {
				return fmt.Errorf("第 %d 行，列 %s：%w", len(rows)+1, c.ID, e)
			}
			r[c.ID] = v
		}
		rows = append(rows, r)
		return nil
	}
	if p.Kind == "json" {
		d := json.NewDecoder(strings.NewReader(text))
		d.UseNumber()
		v, err := decodeJSON(d, 0)
		if err != nil {
			return nil, err
		}
		if _, err = d.Token(); err != io.EOF {
			return nil, fmt.Errorf("JSON 后有多余内容")
		}
		for _, key := range p.Path {
			m, ok := v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("JSON path 不是对象")
			}
			var exists bool
			v, exists = m[key]
			if !exists {
				return nil, fmt.Errorf("JSON 缺少 %s", key)
			}
		}
		switch x := v.(type) {
		case map[string]any:
			err = appendRow(x)
		case []any:
			for _, item := range x {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("JSON 行必须为对象")
				}
				if err = appendRow(m); err != nil {
					break
				}
			}
		default:
			err = fmt.Errorf("JSON 必须是对象或对象数组")
		}
		return rows, err
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var re *regexp.Regexp
	if p.Kind == "regex_rows" {
		re = regexp.MustCompile(p.Pattern)
	}
	kv := map[string]any{}
	header := p.Header
	for index, line := range lines {
		if p.SkipBlankLines && strings.TrimSpace(line) == "" {
			continue
		}
		if header {
			header = false
			if p.Kind == "delimited" {
				values, e := splitColumns(line, p.Delimiter)
				if e != nil || len(values) != len(p.Columns) {
					return nil, fmt.Errorf("表头列数不匹配")
				}
				for i, c := range p.Columns {
					if values[i] != c.ID {
						return nil, fmt.Errorf("表头列 %d 应为 %s", i+1, c.ID)
					}
				}
			}
			continue
		}
		raw := map[string]any{}
		switch p.Kind {
		case "lines":
			raw[p.Columns[0].ID] = line
		case "kv":
			k, v, ok := strings.Cut(line, p.Delimiter)
			k = strings.TrimSpace(k)
			if !ok || k == "" {
				return nil, fmt.Errorf("第 %d 行不是键值", index+1)
			}
			if _, ok = kv[k]; ok {
				return nil, fmt.Errorf("重复键 %s", k)
			}
			kv[k] = strings.TrimSpace(v)
			continue
		case "delimited":
			values, e := splitColumns(line, p.Delimiter)
			if e != nil || len(values) != len(p.Columns) {
				return nil, fmt.Errorf("第 %d 行列数不匹配", index+1)
			}
			for i, c := range p.Columns {
				raw[c.ID] = values[i]
			}
		case "regex_rows":
			m := re.FindStringSubmatch(line)
			if m == nil || m[0] != line {
				return nil, fmt.Errorf("第 %d 行不匹配 regex_rows", index+1)
			}
			for _, c := range p.Columns {
				raw[c.ID] = m[re.SubexpIndex(c.ID)]
			}
		}
		if e := appendRow(raw); e != nil {
			return nil, e
		}
	}
	if p.Kind == "kv" {
		if e := appendRow(kv); e != nil {
			return nil, e
		}
	}
	return rows, nil
}
func splitColumns(line, sep string) ([]string, error) {
	if sep == "whitespace" {
		return strings.Fields(line), nil
	}
	r := csv.NewReader(strings.NewReader(line))
	r.Comma = []rune(sep)[0]
	r.FieldsPerRecord = -1
	return r.Read()
}
func decodeJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 8 {
		return nil, fmt.Errorf("JSON 嵌套过深")
	}
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, e
				}
				s, ok := k.(string)
				if !ok || len(s) > 256 || s == "__proto__" || s == "constructor" || s == "prototype" {
					return nil, fmt.Errorf("非法 JSON 字段")
				}
				if _, ok = m[s]; ok {
					return nil, fmt.Errorf("重复 JSON 字段")
				}
				if len(m) >= 1000 {
					return nil, fmt.Errorf("JSON 字段超限")
				}
				v, e := decodeJSON(d, depth+1)
				if e != nil {
					return nil, e
				}
				m[s] = v
			}
			_, e = d.Token()
			return m, e
		case '[':
			a := []any{}
			for d.More() {
				if len(a) >= MaxRows {
					return nil, fmt.Errorf("JSON 行数超限")
				}
				v, e := decodeJSON(d, depth+1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, e = d.Token()
			return a, e
		}
		return nil, fmt.Errorf("非法 JSON")
	}
	return t, nil
}
