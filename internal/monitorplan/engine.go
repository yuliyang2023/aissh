package monitorplan

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"time"
)

type node struct {
	shape
	value    any
	rows     []map[string]any
	sources  []string
	at       time.Time
	earliest time.Time
	total    int
	reason   string
	waiting  bool
}
type sample struct {
	epoch string
	value float64
	at    time.Time
}
type Engine struct {
	fingerprint string
	previous    map[string][]sample
	Sequence    uint64
	Revision    uint64
	MaxGap      time.Duration
}

func (e *Engine) Clone() *Engine {
	v := *e
	v.previous = map[string][]sample{}
	for k, s := range e.previous {
		v.previous[k] = append([]sample(nil), s...)
	}
	return &v
}

// Evaluate commits temporal baselines only after the complete bounded evaluation.
// Map validation uses a fresh Engine, so retrying validation cannot invent history.
func (e *Engine) Evaluate(ctx context.Context, p Plan, sources []Source, captures map[string]Capture, definition uint64) (Dataset, error) {
	shapes, err := p.shapes(sources)
	if err != nil {
		return Dataset{}, err
	}
	sourceBytes, _ := json.Marshal(sources)
	fingerprint := p.Fingerprint() + Hash(string(sourceBytes))
	next := e.Clone()
	if next.fingerprint != fingerprint {
		next.previous = map[string][]sample{}
		next.Sequence = 0
		next.Revision++
	}
	next.fingerprint = fingerprint
	next.Sequence++
	dataset := Dataset{DefinitionRevision: definition, DataRevision: next.Revision, Sequence: next.Sequence, Metrics: map[string]Metric{}, Tables: map[string]Table{}, Diagnostics: []Diagnostic{}}
	nodes := map[string]node{}
	for _, parser := range p.Parsers {
		if err := ctx.Err(); err != nil {
			return Dataset{}, err
		}
		c, ok := captures[parser.SourceID]
		n := node{shape: shapes[parser.ID], sources: []string{parser.SourceID}, at: c.FinishedAt, earliest: c.FinishedAt}
		if !ok || !c.Good() {
			n.reason = "采集不完整或失败"
			if ok {
				n.reason += "（" + c.Status + "）"
				if c.Stderr != nil && *c.Stderr != "" {
					n.reason += "：" + clip(*c.Stderr, 400)
				} else if c.Streams == "merged" && c.Stdout != "" {
					n.reason += "：" + clip(c.Stdout, 400)
				}
			}
			dataset.Diagnostics = append(dataset.Diagnostics, Diagnostic{Stage: "capture", SourceID: parser.SourceID, NodeID: parser.ID, Message: n.reason})
		} else {
			n.rows, err = parse(parser, c.Stdout)
			if err != nil {
				n.reason = err.Error()
				dataset.Diagnostics = append(dataset.Diagnostics, Diagnostic{Stage: "parse", SourceID: parser.SourceID, NodeID: parser.ID, Message: n.reason})
			}
			n.total = len(n.rows)
		}
		nodes[parser.ID] = n
	}
	order, _ := p.ordered()
	for _, step := range order {
		if err := ctx.Err(); err != nil {
			return Dataset{}, err
		}
		n := nodes[step.Input]
		other := nodes[step.Other]
		n.waiting = false
		n.shape = shapes[step.ID]
		n.sources = append([]string(nil), n.sources...)
		if step.Other != "" {
			for _, id := range other.sources {
				found := false
				for _, s := range n.sources {
					found = found || s == id
				}
				if !found {
					n.sources = append(n.sources, id)
				}
			}
			if other.at.After(n.at) {
				n.at = other.at
			}
			if other.earliest.Before(n.earliest) {
				n.earliest = other.earliest
			}
			if other.reason != "" {
				n.reason = other.reason
			}
		}
		if n.reason == "" {
			n, err = next.transform(step, n, nodes[step.Input], other)
			if err != nil {
				n.reason = err.Error()
			}
		}
		if n.reason != "" {
			n.rows = nil
			n.value = nil
			if !n.waiting {
				delete(next.previous, step.ID)
			}
			dataset.Diagnostics = append(dataset.Diagnostics, Diagnostic{Stage: "transform", NodeID: step.ID, Message: n.reason})
		}
		if n.typ != "table" {
			n.rows = nil
			n.total = 0
		}
		encoded, er := json.Marshal(n.rows)
		if er != nil || len(encoded) > MaxRaw {
			return Dataset{}, fmt.Errorf("节点 %s 中间数据超过 256 KiB", step.ID)
		}
		nodes[step.ID] = n
	}
	for _, o := range p.Outputs {
		n := nodes[o.Input]
		quality := "ok"
		if n.reason != "" {
			quality = "unknown"
		}
		var observed *time.Time
		if !n.at.IsZero() {
			t := n.at
			observed = &t
		}
		if o.Kind == "metric" {
			dataset.Metrics[o.ID] = Metric{Value: n.value, Unit: o.Unit, Quality: quality, ObservedAt: observed, SourceIDs: n.sources, Reason: n.reason}
		} else {
			var total, omitted *int
			if n.reason == "" {
				t := n.total
				v := max(0, t-len(n.rows))
				total = &t
				omitted = &v
			}
			dataset.Tables[o.ID] = Table{Columns: n.columns, Rows: n.rows, Quality: quality, ObservedAt: observed, SourceIDs: n.sources, TotalRows: total, ShownRows: len(n.rows), OmittedRows: omitted, Reason: n.reason}
		}
	}
	b, err := json.Marshal(dataset)
	if err != nil || len(b) > MaxDataset {
		return Dataset{}, fmt.Errorf("统一数据集超过 64 KiB，请使用 project/limit 缩小展示数据")
	}
	*e = *next
	return dataset, nil
}
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
func (e *Engine) transform(s Step, out, n, other node) (node, error) {
	fail := func(message string) (node, error) { return out, fmt.Errorf("%s", message) }
	switch s.Op {
	case "filter":
		out.rows = []map[string]any{}
		var re *regexp.Regexp
		if s.Test == "regex" {
			re = regexp.MustCompile(s.Pattern)
		}
		c, _ := n.column(s.Field)
		for _, r := range n.rows {
			v := r[s.Field]
			keep := false
			switch s.Test {
			case "equals", "in":
				for _, item := range s.Values {
					x, _ := convert(item, c.Type)
					keep = keep || reflect.DeepEqual(v, x)
				}
			case "range":
				x := v.(float64)
				keep = (s.Min == nil || x >= *s.Min) && (s.Max == nil || x <= *s.Max)
			case "regex":
				keep = re.MatchString(v.(string))
			}
			if keep {
				out.rows = append(out.rows, r)
			}
		}
		if n.total != len(n.rows) {
			return fail("不能对已截短的表进行筛选")
		}
		out.total = len(out.rows)
	case "sort":
		out.rows = append([]map[string]any(nil), n.rows...)
		sort.SliceStable(out.rows, func(i, j int) bool {
			a, b := out.rows[i][s.Field], out.rows[j][s.Field]
			var cmp int
			switch x := a.(type) {
			case float64:
				y := b.(float64)
				if x < y {
					cmp = -1
				} else if x > y {
					cmp = 1
				}
			case string:
				y := b.(string)
				if x < y {
					cmp = -1
				} else if x > y {
					cmp = 1
				}
			case bool:
				if x != b.(bool) {
					if x {
						cmp = 1
					} else {
						cmp = -1
					}
				}
			}
			if s.Descending {
				return cmp > 0
			}
			return cmp < 0
		})
	case "limit":
		if len(out.rows) > s.Limit {
			out.rows = out.rows[:s.Limit]
		}
	case "project", "rename":
		out.rows = make([]map[string]any, 0, len(n.rows))
		for _, r := range n.rows {
			v := map[string]any{}
			if s.Op == "project" {
				for _, f := range s.Fields {
					v[f] = r[f]
				}
			} else {
				for key, x := range r {
					k := key
					for _, pair := range s.Names {
						if pair.From == key {
							k = pair.To
							break
						}
					}
					v[k] = x
				}
			}
			out.rows = append(out.rows, v)
		}
	case "concat":
		if n.total != len(n.rows) || other.total != len(other.rows) {
			return fail("concat 不能使用截短的表")
		}
		if len(n.rows)+len(other.rows) > MaxRows {
			return fail("concat 行数超限")
		}
		out.rows = append(append([]map[string]any{}, n.rows...), other.rows...)
		out.total = len(out.rows)
	case "join":
		if out.at.Sub(out.earliest) > time.Duration(s.MaxSkewMS)*time.Millisecond {
			return fail("跨源采样时间差超过 maxSkewMs")
		}
		if n.total != len(n.rows) || other.total != len(other.rows) {
			return fail("join 不能使用截短的表")
		}
		index := map[any][]map[string]any{}
		for _, r := range other.rows {
			index[r[s.OtherKey]] = append(index[r[s.OtherKey]], r)
		}
		out.rows = []map[string]any{}
		for _, r := range n.rows {
			for _, b := range index[r[s.Key]] {
				if len(out.rows) >= MaxRows {
					return fail("join 行数超限")
				}
				v := map[string]any{}
				for k, x := range r {
					v[k] = x
				}
				for k, x := range b {
					if k != s.OtherKey {
						v[k] = x
					}
				}
				out.rows = append(out.rows, v)
			}
		}
		out.total = len(out.rows)
	case "count", "sum", "min", "max", "avg", "first":
		if n.total != len(n.rows) {
			return fail("不能汇总截短的表")
		}
		if s.Op == "count" {
			out.value = float64(n.total)
			break
		}
		if s.Op == "first" {
			if len(n.rows) != 1 {
				return fail("first 要求恰好一行，请先筛选唯一记录")
			}
			out.value = n.rows[0][s.Field]
			break
		}
		if len(n.rows) == 0 && s.Op != "sum" {
			return fail("空集合没有该汇总值")
		}
		value := 0.0
		for i, r := range n.rows {
			x := r[s.Field].(float64)
			switch s.Op {
			case "sum", "avg":
				value += x
			case "min":
				if i == 0 || x < value {
					value = x
				}
			case "max":
				if i == 0 || x > value {
					value = x
				}
			}
			if !finite(value) {
				return fail("汇总数值越界")
			}
		}
		if s.Op == "avg" {
			value /= float64(len(n.rows))
		}
		out.value = value
	case "convert_unit":
		out.value = n.value.(float64) * units[s.From].scale / units[s.To].scale
	case "convert_type":
		v, err := convert(n.value, s.To)
		if err != nil {
			return out, err
		}
		out.value = v
	case "enum":
		v, err := convert(n.value, "string")
		if err != nil {
			return out, err
		}
		found := false
		for _, pair := range s.Names {
			if pair.From == v {
				out.value = pair.To
				found = true
				break
			}
		}
		if !found {
			return fail("枚举没有对应映射")
		}
	case "add", "subtract", "multiply", "divide":
		a := n.value.(float64)
		var b float64
		if s.Other != "" {
			b = other.value.(float64)
		} else {
			b = *s.Value
		}
		switch s.Op {
		case "add":
			out.value = a + b
		case "subtract":
			out.value = a - b
		case "multiply":
			out.value = a * b
		case "divide":
			if b == 0 {
				return fail("除数为零")
			}
			out.value = a / b
		}
	case "delta", "rate", "rolling_avg":
		current := sample{value: n.value.(float64), at: n.at}
		if s.Other != "" {
			current.epoch = fmt.Sprint(other.value)
		}
		prior := e.previous[s.ID]
		e.previous[s.ID] = []sample{current}
		if len(prior) == 0 {
			return temporalGap(out, "等待第二个有效采样")
		}
		last := prior[len(prior)-1]
		if current.epoch != last.epoch {
			return temporalGap(out, "服务实例变化，重新建立基线")
		}
		duration := current.at.Sub(last.at)
		gap := e.MaxGap
		if gap <= 0 {
			gap = 15 * time.Second
		}
		if duration <= 0 || duration > gap {
			return temporalGap(out, "采样间隔中断，重新建立基线")
		}
		if s.Op == "rolling_avg" {
			prior = append(prior, current)
			if len(prior) > s.Window {
				prior = prior[len(prior)-s.Window:]
			}
			e.previous[s.ID] = prior
			if len(prior) < s.Window {
				return temporalGap(out, "等待滚动窗口有效采样")
			}
			sum := 0.0
			for _, v := range prior {
				sum += v.value
			}
			out.value = sum / float64(len(prior))
		} else {
			delta := current.value - last.value
			if s.Semantics == "counter" && delta < 0 {
				return temporalGap(out, "计数器回退，重新建立基线")
			}
			if s.Op == "rate" {
				delta /= duration.Seconds()
			}
			out.value = delta
		}
	}
	if value, ok := out.value.(float64); ok && !finite(value) {
		return fail("数值非有限或超出安全整数范围")
	}
	return out, nil
}

// Temporal gaps retain the new baseline; real failures clear it in Evaluate.
func temporalGap(n node, reason string) (node, error) {
	n.value = nil
	n.reason = reason
	n.waiting = true
	return n, nil
}

// Failure invalidates temporal baselines and emits an explicit history gap even
// when evaluation itself exceeded a budget.
func (e *Engine) Failure(p Plan, sources []Source, definition uint64, message string) Dataset {
	e.previous = map[string][]sample{}
	e.Sequence++
	d := Dataset{DefinitionRevision: definition, DataRevision: e.Revision, Sequence: e.Sequence, Metrics: map[string]Metric{}, Tables: map[string]Table{}, Diagnostics: []Diagnostic{{Stage: "transform", Message: message}}}
	shapes, _ := p.shapes(sources)
	ids := []string{}
	for _, s := range sources {
		ids = append(ids, s.ID)
	}
	for _, o := range p.Outputs {
		if o.Kind == "metric" {
			d.Metrics[o.ID] = Metric{Unit: o.Unit, Quality: "unknown", SourceIDs: ids, Reason: message}
		} else {
			d.Tables[o.ID] = Table{Columns: shapes[o.Input].columns, Quality: "unknown", SourceIDs: ids, Reason: message}
		}
	}
	return d
}
