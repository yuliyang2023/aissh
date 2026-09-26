package widgets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

func Parse(c Contract, text string) (map[string]any, []string, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	if len(text) > MaxSnapshot {
		return nil, nil, errors.New("结构化快照超过 64 KiB")
	}
	raw := map[string]any{}
	switch c.Format {
	case "json":
		d := json.NewDecoder(strings.NewReader(text))
		d.UseNumber()
		v, err := decode(d, 0)
		if err != nil {
			return nil, nil, err
		}
		if _, err = d.Token(); err != io.EOF {
			return nil, nil, errors.New("JSON 后有多余内容")
		}
		var ok bool
		raw, ok = v.(map[string]any)
		if !ok {
			return nil, nil, errors.New("JSON 必须是对象")
		}
	case "kv":
		for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
			k, v, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "=")
			if !ok || !fieldName.MatchString(k) {
				return nil, nil, errors.New("键值输出必须为每行 NAME=value")
			}
			if _, ok = raw[k]; ok {
				return nil, nil, fmt.Errorf("重复字段 %s", k)
			}
			raw[k] = v
		}
	case "text":
		raw[c.Fields[0].Name] = text
	}
	result := map[string]any{}
	issues := []string{}
	for _, f := range c.Fields {
		v, ok := raw[f.Name]
		delete(raw, f.Name)
		if !ok || v == nil {
			if !f.Nullable {
				return nil, nil, fmt.Errorf("缺少字段 %s", f.Name)
			}
			result[f.Name] = nil
			issues = append(issues, f.Name+": missing")
			continue
		}
		var err error
		switch f.Type {
		case "number":
			var n float64
			switch value := v.(type) {
			case json.Number:
				n, err = value.Float64()
			case string:
				if c.Format != "kv" {
					err = errors.New("不是数值")
				} else {
					n, err = strconv.ParseFloat(value, 64)
				}
			default:
				err = errors.New("不是数值")
			}
			if math.IsInf(n, 0) || math.IsNaN(n) || math.Abs(n) > 9007199254740991 {
				err = errors.New("数值越界，请使用 string 传输大整数")
			}
			v = n
		case "string":
			if _, ok := v.(string); !ok {
				err = errors.New("不是字符串")
			}
		case "boolean":
			if c.Format == "kv" {
				s, _ := v.(string)
				if s == "true" {
					v = true
				} else if s == "false" {
					v = false
				} else {
					err = errors.New("不是布尔值")
				}
			} else if _, ok := v.(bool); !ok {
				err = errors.New("不是布尔值")
			}
		case "table":
			v, err = table(v)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("字段 %s: %w", f.Name, err)
		}
		result[f.Name] = v
	}
	if len(raw) > 0 {
		return nil, nil, errors.New("输出包含未声明字段")
	}
	return result, issues, nil
}
func decode(d *json.Decoder, depth int) (any, error) {
	if depth > 6 {
		return nil, errors.New("JSON 嵌套过深")
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := t.(json.Delim); ok {
		switch delimiter {
		case '{':
			m := map[string]any{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, err
				}
				k, ok := key.(string)
				if !ok || k == "__proto__" || k == "constructor" || k == "prototype" || len(k) > 64 {
					return nil, errors.New("非法 JSON 字段")
				}
				if _, ok = m[k]; ok {
					return nil, errors.New("重复 JSON 字段")
				}
				if len(m) >= 50 {
					return nil, errors.New("字段过多")
				}
				v, err := decode(d, depth+1)
				if err != nil {
					return nil, err
				}
				m[k] = v
			}
			_, err = d.Token()
			return m, err
		case '[':
			a := []any{}
			for d.More() {
				if len(a) >= 1000 {
					return nil, errors.New("表格超过 1000 行")
				}
				v, err := decode(d, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			_, err = d.Token()
			return a, err
		default:
			return nil, errors.New("非法 JSON")
		}
	}
	return t, nil
}
func table(v any) (any, error) {
	rows, ok := v.([]any)
	if !ok {
		return nil, errors.New("table 必须为对象数组")
	}
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, errors.New("表格行必须是对象")
		}
		for k, cell := range row {
			switch n := cell.(type) {
			case nil, string, bool:
			case json.Number:
				f, err := n.Float64()
				if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 9007199254740991 {
					return nil, errors.New("表格数值越界")
				}
				row[k] = f
			default:
				return nil, errors.New("表格只支持标量单元格")
			}
		}
	}
	return rows, nil
}
func Record(previous Data, c Contract, content string, collectErr error, now time.Time) Data {
	next := previous
	next.Sequence++
	next.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	next.Error = ""
	next.Issues = []string{}
	next.Data = nil
	if collectErr == nil {
		next.Data, next.Issues, collectErr = Parse(c, content)
	}
	if collectErr != nil {
		next.Error = collectErr.Error()
	} else if len(next.Issues) == 0 {
		next.LastGood = next.Data
		next.LastGoodAt = next.UpdatedAt
	}
	values := map[string]*float64{}
	for _, f := range c.Fields {
		if f.History {
			var ptr *float64
			if n, ok := next.Data[f.Name].(float64); ok {
				x := n
				ptr = &x
			}
			values[f.Name] = ptr
		}
	}
	// A gap is explicit even if the previous successful sample was never displayed.
	history := append([]Point(nil), previous.History...)
	if len(values) > 0 {
		if len(history) > 0 {
			last, _ := time.Parse(time.RFC3339Nano, history[len(history)-1].Time)
			gap := now.Sub(last)
			if gap < 0 || gap > time.Duration(max(15000, next.IntervalMS*3))*time.Millisecond {
				history = append(history, Point{Time: last.Add(time.Millisecond).UTC().Format(time.RFC3339Nano), Values: map[string]*float64{}})
			}
		}
		history = append(history, Point{Time: next.UpdatedAt, Values: values})
	}
	for len(history) > 0 {
		t, _ := time.Parse(time.RFC3339Nano, history[0].Time)
		b, _ := json.Marshal(history)
		if len(history) <= 600 && len(b) <= MaxHistory && now.Sub(t) <= 10*time.Minute {
			break
		}
		history = history[1:]
		next.Truncated = true
	}
	next.History = history
	return next
}

// Redact clones values field-by-field, preserving JSON validity and null semantics.
func Redact(data Data, redact func(string) string) Data {
	b, _ := json.Marshal(data)
	var copy Data
	_ = json.Unmarshal(b, &copy)
	var clean func(any, string) any
	clean = func(v any, path string) any {
		switch x := v.(type) {
		case string:
			if redact(x) != x {
				copy.Issues = append(copy.Issues, path+": redacted")
				return nil
			}
		case map[string]any:
			for k, v := range x {
				x[k] = clean(v, path+"."+k)
			}
		case []any:
			for i, v := range x {
				x[i] = clean(v, path)
			}
		}
		return v
	}
	if copy.Data != nil {
		clean(copy.Data, "data")
	}
	if copy.LastGood != nil {
		clean(copy.LastGood, "lastGood")
	}
	copy.Error = redact(copy.Error)
	return copy
}
