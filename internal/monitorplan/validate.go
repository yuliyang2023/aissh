package monitorplan

import (
	"fmt"
	"reflect"
	"regexp"
)

type shape struct {
	columns   []Column
	typ, unit string
}

func (s shape) column(id string) (Column, error) {
	for _, c := range s.columns {
		if c.ID == id {
			return c, nil
		}
	}
	return Column{}, fmt.Errorf("缺少列 %q", id)
}
func (p Plan) ordered() ([]Step, error) {
	seen := map[string]bool{}
	for _, v := range p.Parsers {
		if !ValidID(v.ID) || seen[v.ID] {
			return nil, fmt.Errorf("重复或无效节点 %q", v.ID)
		}
		seen[v.ID] = true
	}
	all := map[string]bool{}
	for k := range seen {
		all[k] = true
	}
	for _, s := range p.Steps {
		if !ValidID(s.ID) || all[s.ID] {
			return nil, fmt.Errorf("重复或无效节点 %q", s.ID)
		}
		all[s.ID] = true
	}
	pending := append([]Step(nil), p.Steps...)
	out := []Step{}
	for len(pending) > 0 {
		changed := false
		next := []Step{}
		for _, s := range pending {
			if !all[s.Input] || (s.Other != "" && !all[s.Other]) {
				return nil, fmt.Errorf("节点 %s 引用不存在", s.ID)
			}
			if seen[s.Input] && (s.Other == "" || seen[s.Other]) {
				out = append(out, s)
				seen[s.ID] = true
				changed = true
			} else {
				next = append(next, s)
			}
		}
		if !changed {
			return nil, fmt.Errorf("数据计划存在循环引用")
		}
		pending = next
	}
	return out, nil
}
func (p Plan) Validate(sources []Source) error { _, err := p.shapes(sources); return err }
func (p Plan) shapes(sources []Source) (map[string]shape, error) {
	fail := func(s string) (map[string]shape, error) { return nil, fmt.Errorf("数据计划：%s", s) }
	if len(p.Parsers) == 0 || len(p.Parsers) > 16 || len(p.Steps) > 32 || len(p.Outputs) == 0 || len(p.Outputs) > 50 {
		return fail("解析器/转换/输出数量超限")
	}
	src := map[string]bool{}
	for _, s := range sources {
		src[s.ID] = true
	}
	nodes := map[string]shape{}
	order, err := p.ordered()
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, v := range p.Parsers {
		if !src[v.SourceID] {
			return fail("未知数据源 " + v.SourceID)
		}
		used[v.SourceID] = true
		if len(v.Columns) == 0 || len(v.Columns) > 50 {
			return fail("解析器需要 1–50 列")
		}
		keys := map[string]bool{}
		for _, c := range v.Columns {
			if !ValidID(c.ID) || keys[c.ID] || (c.Type != "string" && c.Type != "number" && c.Type != "boolean") || len(c.Unit) > 32 {
				return fail("无效列 " + c.ID)
			}
			keys[c.ID] = true
		}
		if v.OnUnmatched != "" && v.OnUnmatched != "error" {
			return fail("不匹配行必须报错；只允许显式跳过表头或空行")
		}
		switch v.Kind {
		case "lines":
			if len(v.Columns) != 1 || v.Columns[0].Type != "string" {
				return fail("lines 需要单个 string 列")
			}
		case "kv":
			if v.Delimiter == "" {
				return fail("kv 必须声明 delimiter")
			}
		case "delimited":
			if v.Delimiter != "whitespace" && len([]rune(v.Delimiter)) != 1 {
				return fail("delimited 使用单字符或 whitespace 分隔符")
			}
		case "regex_rows":
			r, e := regexp.Compile(v.Pattern)
			if e != nil || len(v.Pattern) > 4096 {
				return fail("无效 RE2 表达式")
			}
			for _, c := range v.Columns {
				if r.SubexpIndex(c.ID) < 1 {
					return fail("正则缺少命名分组 " + c.ID)
				}
			}
		case "json":
			if len(v.Path) > 6 {
				return fail("JSON path 过深")
			}
		default:
			return fail("不支持解析器 " + v.Kind)
		}
		nodes[v.ID] = shape{columns: v.Columns, typ: "table"}
	}
	for _, s := range sources {
		if !used[s.ID] {
			return fail("未使用数据源 " + s.ID)
		}
	}
	for _, s := range order {
		n := nodes[s.Input]
		other := nodes[s.Other]
		result := n
		table := func() error {
			if n.typ != "table" {
				return fmt.Errorf("%s 需要表输入", s.ID)
			}
			return nil
		}
		numeric := func() error {
			if n.typ != "number" {
				return fmt.Errorf("%s 需要数值输入", s.ID)
			}
			return nil
		}
		var e error
		switch s.Op {
		case "filter", "sort":
			e = table()
			c, ce := n.column(s.Field)
			if e == nil {
				e = ce
			}
			if s.Op == "filter" {
				switch s.Test {
				case "equals", "in":
					if len(s.Values) == 0 || len(s.Values) > 100 {
						e = fmt.Errorf("筛选枚举数量无效")
					}
					for _, v := range s.Values {
						if _, err := convert(v, c.Type); err != nil {
							e = err
						}
					}
				case "range":
					if c.Type != "number" || s.Min == nil && s.Max == nil {
						e = fmt.Errorf("range 需要数值列和边界")
					}
					if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
						e = fmt.Errorf("range 边界倒置")
					}
				case "regex":
					if _, er := regexp.Compile(s.Pattern); er != nil || len(s.Pattern) > 4096 || c.Type != "string" {
						e = fmt.Errorf("无效字符串 RE2 筛选")
					}
				default:
					e = fmt.Errorf("未知 filter test %s", s.Test)
				}
			}
		case "project":
			e = table()
			result.columns = nil
			for _, f := range s.Fields {
				c, er := n.column(f)
				if er != nil {
					e = er
				}
				result.columns = append(result.columns, c)
			}
			if len(s.Fields) == 0 {
				e = fmt.Errorf("project 字段为空")
			}
		case "rename":
			e = table()
			result.columns = append([]Column(nil), n.columns...)
			for _, pair := range s.Names {
				if _, er := n.column(pair.From); er != nil {
					e = er
				}
				for i := range result.columns {
					if result.columns[i].ID == pair.From {
						result.columns[i].ID = pair.To
					}
				}
			}
		case "concat":
			e = table()
			if other.typ != "table" || !reflect.DeepEqual(n.columns, other.columns) {
				e = fmt.Errorf("concat 列不一致")
			}
		case "join":
			e = table()
			a, ea := n.column(s.Key)
			b, eb := other.column(s.OtherKey)
			if ea != nil || eb != nil || a.Type != b.Type || a.Unit != b.Unit || s.MaxSkewMS <= 0 || s.MaxSkewMS > 60000 {
				e = fmt.Errorf("join 必须声明兼容键和 1–60000 ms 最大时间差")
			}
			result.columns = append([]Column(nil), n.columns...)
			for _, c := range other.columns {
				if c.ID == s.OtherKey {
					continue
				}
				if _, er := n.column(c.ID); er == nil {
					e = fmt.Errorf("join 列冲突，请先 rename")
				}
				result.columns = append(result.columns, c)
			}
		case "limit":
			e = table()
			if s.Limit < 1 || s.Limit > MaxRows {
				e = fmt.Errorf("limit 范围 1–1000")
			}
		case "count":
			e = table()
			result = shape{typ: "number"}
		case "sum", "min", "max", "avg":
			e = table()
			c, er := n.column(s.Field)
			if er != nil || c.Type != "number" {
				e = fmt.Errorf("汇总需要数值列 %s", s.Field)
			}
			result = shape{typ: "number", unit: c.Unit}
		case "first":
			e = table()
			c, er := n.column(s.Field)
			if er != nil {
				e = er
			}
			result = shape{typ: c.Type, unit: c.Unit}
		case "convert_unit":
			e = numeric()
			if s.From != n.unit || !compatibleUnits(s.From, s.To) {
				e = fmt.Errorf("单位不能转换：%s → %s（输入 %s）", s.From, s.To, n.unit)
			}
			result.unit = s.To
		case "convert_type":
			if n.typ == "table" || (s.To != "string" && s.To != "number" && s.To != "boolean") {
				e = fmt.Errorf("无效标量类型转换")
			}
			result.typ = s.To
		case "enum":
			if n.typ == "table" || len(s.Names) == 0 {
				e = fmt.Errorf("enum 需要标量及 names")
			}
			result = shape{typ: "string"}
		case "add", "subtract", "multiply", "divide":
			e = numeric()
			if s.Other != "" {
				if other.typ != "number" {
					e = fmt.Errorf("运算需要数值")
				}
				if (s.Op == "add" || s.Op == "subtract") && other.unit != n.unit {
					e = fmt.Errorf("加减单位不一致")
				}
				if s.Op == "multiply" && other.unit != "" {
					e = fmt.Errorf("乘数必须无量纲")
				}
				if s.Op == "divide" && other.unit != "" {
					if other.unit != n.unit {
						e = fmt.Errorf("除数单位不兼容")
					} else {
						result.unit = ""
					}
				}
			} else if s.Value == nil {
				e = fmt.Errorf("运算需要 other 或 value")
			}
		case "delta", "rate":
			e = numeric()
			if s.Other != "" && other.typ == "table" {
				e = fmt.Errorf("实例标识 other 必须为标量")
			}
			if s.Semantics != "counter" && s.Semantics != "gauge" {
				e = fmt.Errorf("delta/rate 必须声明 counter 或 gauge")
			}
			if s.Op == "rate" {
				result.unit = n.unit + "/s"
			}
		case "rolling_avg":
			e = numeric()
			if s.Other != "" && other.typ == "table" {
				e = fmt.Errorf("实例标识 other 必须为标量")
			}
			if s.Window < 2 || s.Window > 600 {
				e = fmt.Errorf("rolling_avg window 范围 2–600")
			}
		default:
			e = fmt.Errorf("尚不支持算子 %q", s.Op)
		}
		if e != nil {
			return nil, fmt.Errorf("节点 %s：%w", s.ID, e)
		}
		keys := map[string]bool{}
		for _, c := range result.columns {
			if !ValidID(c.ID) || keys[c.ID] {
				return fail("重复或无效结果列 " + c.ID)
			}
			keys[c.ID] = true
		}
		nodes[s.ID] = result
	}
	ids := map[string]bool{}
	hist := 0
	for _, o := range p.Outputs {
		n, ok := nodes[o.Input]
		if !ValidID(o.ID) || ids[o.ID] || !ok || (o.Kind != "metric" && o.Kind != "table") || ((o.Kind == "table") != (n.typ == "table")) {
			return fail("无效输出 " + o.ID)
		}
		ids[o.ID] = true
		if n.unit != "" && o.Unit != n.unit {
			return fail("输出单位不一致 " + o.ID)
		}
		if o.History {
			hist++
			if n.typ != "number" {
				return fail("历史只接受数值指标")
			}
		}
	}
	if hist > 20 {
		return fail("最多 20 个历史指标")
	}
	return nodes, nil
}

var units = map[string]struct {
	family string
	scale  float64
}{"B": {"bytes", 1}, "KiB": {"bytes", 1024}, "MiB": {"bytes", 1048576}, "GiB": {"bytes", 1073741824}, "KB": {"bytes", 1000}, "MB": {"bytes", 1e6}, "GB": {"bytes", 1e9}, "s": {"time", 1}, "ms": {"time", .001}, "us": {"time", .000001}, "ratio": {"ratio", 1}, "%": {"ratio", .01}}

func compatibleUnits(a, b string) bool {
	x, ok := units[a]
	y, ok2 := units[b]
	return ok && ok2 && x.family == y.family
}
