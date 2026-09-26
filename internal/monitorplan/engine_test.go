package monitorplan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func processFixture() ([]Source, Plan, map[string]Capture) {
	sources := []Source{{ID: "ps", Command: "LC_ALL=C ps -e -o pid= -o comm= -o rss=", Required: true, TimeoutMS: 8000}}
	plan := Plan{Parsers: []Parser{{ID: "processes", SourceID: "ps", Kind: "regex_rows", Pattern: `^\s*(?P<pid>[0-9]+)\s+(?P<name>.+?)\s+(?P<rssKiB>[0-9]+)\s*$`, Columns: []Column{{ID: "pid", Type: "string"}, {ID: "name", Type: "string"}, {ID: "rssKiB", Type: "number", Unit: "KiB"}}, SkipBlankLines: true}}, Steps: []Step{{ID: "nginx", Op: "filter", Input: "processes", Field: "name", Test: "in", Values: []string{"nginx", "nginx-debug"}}, {ID: "count", Op: "count", Input: "nginx"}, {ID: "rss", Op: "sum", Input: "nginx", Field: "rssKiB"}, {ID: "memory", Op: "convert_unit", Input: "rss", From: "KiB", To: "MiB"}}, Outputs: []Output{{ID: "processCount", Kind: "metric", Input: "count", History: true}, {ID: "rssMiB", Kind: "metric", Input: "memory", Unit: "MiB", History: true}, {ID: "processList", Kind: "table", Input: "nginx"}}}
	zero := 0
	now := time.Now()
	captures := map[string]Capture{"ps": {ExitCode: &zero, Status: "ok", Complete: true, StartedAt: now, FinishedAt: now, Stdout: "1 init 1024\n1823 nginx 20480\n1824 nginx 10240\n2000 other process 4096\n"}}
	return sources, plan, captures
}
func TestProcessMappingAndUnknownSemantics(t *testing.T) {
	s, p, c := processFixture()
	e := &Engine{}
	d, err := e.Evaluate(context.Background(), p, s, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Metrics["processCount"].Value != float64(2) || d.Metrics["rssMiB"].Value != float64(30) || len(d.Tables["processList"].Rows) != 2 {
		t.Fatalf("wrong dataset %+v", d)
	}
	for _, test := range []struct {
		name, raw, status string
		want              any
	}{{"empty", "", "ok", float64(0)}, {"no_nginx", "1 init 1024", "ok", float64(0)}, {"bad_columns", "1 init", "ok", nil}, {"diagnostic", "1 init 1024\npermission denied", "ok", nil}, {"missing_tool", "", "exit_error", nil}, {"truncated", "1823 nginx 20480", "too_large", nil}} {
		t.Run(test.name, func(t *testing.T) {
			v := c["ps"]
			v.Stdout = test.raw
			v.Status = test.status
			c["ps"] = v
			d, err := e.Evaluate(context.Background(), p, s, c, 2)
			if err != nil {
				t.Fatal(err)
			}
			if d.Metrics["processCount"].Value != test.want {
				t.Fatalf("failure fabricated data: %+v", d)
			}
			if test.want == nil && len(d.Diagnostics) == 0 {
				t.Fatal("missing stage diagnostics")
			}
		})
	}
}
func TestRateUsesRealElapsedTimeAndBreaksOnFailureResetAndGap(t *testing.T) {
	s := []Source{{ID: "counter", Required: true, Command: "status", TimeoutMS: 1000}}
	p := Plan{Parsers: []Parser{{ID: "raw", SourceID: "counter", Kind: "kv", Delimiter: "=", Columns: []Column{{ID: "requests", Type: "number"}}}}, Steps: []Step{{ID: "total", Op: "first", Input: "raw", Field: "requests"}, {ID: "qps", Op: "rate", Input: "total", Semantics: "counter"}}, Outputs: []Output{{ID: "qps", Kind: "metric", Input: "qps", Unit: "/s", History: true}}}
	now := time.Now()
	zero := 0
	e := &Engine{MaxGap: 15 * time.Second}
	value := 20.0
	want5 := 5.0
	tests := []struct {
		sec         int
		raw, status string
		want        *float64
	}{{0, "requests=100", "ok", nil}, {5, "requests=200", "ok", &value}, {10, "", "timeout", nil}, {15, "requests=300", "ok", nil}, {20, "requests=400", "ok", &value}, {25, "requests=10", "ok", nil}, {30, "requests=35", "ok", &want5}, {60, "requests=1000", "ok", nil}, {65, "requests=1100", "ok", &value}}
	for _, v := range tests {
		at := now.Add(time.Duration(v.sec) * time.Second)
		c := map[string]Capture{"counter": {Stdout: v.raw, Status: v.status, Complete: true, ExitCode: &zero, FinishedAt: at}}
		d, err := e.Evaluate(context.Background(), p, s, c, 1)
		if err != nil {
			t.Fatal(err)
		}
		got := d.Metrics["qps"].Value
		if v.want == nil && got != nil || v.want != nil && got != *v.want {
			t.Fatalf("second=%d got=%v want=%v diagnostics=%+v", v.sec, got, v.want, d.Diagnostics)
		}
	}
}
func TestPlanValidationAndBounds(t *testing.T) {
	s, p, c := processFixture()
	for _, name := range []string{"cycle", "unit", "missing", "regex", "field", "binding"} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(p)
			var q Plan
			_ = json.Unmarshal(data, &q)
			switch name {
			case "cycle":
				q.Steps[0].Input = "count"
			case "unit":
				q.Steps[3].To = "ms"
			case "missing":
				q.Steps[0].Input = "nope"
			case "regex":
				q.Parsers[0].Pattern = "["
			case "field":
				q.Steps[1] = Step{ID: "count", Op: "sum", Input: "nginx", Field: "bad"}
			case "binding":
				if ValidateBindings(q, []Binding{{Prop: "__proto__", Kind: "metric", OutputID: "processCount"}}) == nil {
					t.Fatal("unsafe binding")
				}
				return
			}
			if q.Validate(s) == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
	v := c["ps"]
	v.Stdout = strings.Repeat("1 nginx 1024\n", 1001)
	c["ps"] = v
	d, err := (&Engine{}).Evaluate(context.Background(), p, s, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Metrics["processCount"].Value != nil {
		t.Fatal("truncated count accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Engine{}).Evaluate(ctx, p, s, c, 1); err == nil {
		t.Fatal("cancel ignored")
	}
}
func TestParsersRejectMalformedDataAndPreserveLargeIntegerStrings(t *testing.T) {
	for _, v := range []struct {
		p   Parser
		raw string
		bad bool
	}{
		{Parser{Kind: "json", Columns: []Column{{ID: "n", Type: "string"}}}, `{"n":9007199254740993}`, false},
		{Parser{Kind: "json", Columns: []Column{{ID: "n", Type: "number"}}}, `{"n":9007199254740993}`, true},
		{Parser{Kind: "json", Columns: []Column{{ID: "n", Type: "string"}}}, `{"n":"1","n":"2"}`, true},
		{Parser{Kind: "json", Columns: []Column{{ID: "n", Type: "number"}}}, `{"n":1} junk`, true},
		{Parser{Kind: "delimited", Delimiter: ",", Header: true, Columns: []Column{{ID: "name", Type: "string"}, {ID: "count", Type: "number"}}}, "name,count\n\"a,b\",2", false},
		{Parser{Kind: "delimited", Delimiter: "whitespace", Columns: []Column{{ID: "name", Type: "string"}, {ID: "count", Type: "number"}}}, "nginx 2 unexpected", true},
		{Parser{Kind: "kv", Delimiter: "=", Columns: []Column{{ID: "n", Type: "number"}}}, "n=1\nn=2", true},
	} {
		rows, err := parse(v.p, v.raw)
		if (err != nil) != v.bad {
			t.Fatalf("raw=%q rows=%v err=%v", v.raw, rows, err)
		}
	}
}
func TestJoinSkewAndLimitDoNotInventTotals(t *testing.T) {
	s, p, c := processFixture()
	s = append(s, Source{ID: "other", Command: "status", Required: true, TimeoutMS: 1000})
	p.Parsers = append(p.Parsers, Parser{ID: "state", SourceID: "other", Kind: "kv", Delimiter: "=", Columns: []Column{{ID: "pid", Type: "string"}, {ID: "state", Type: "string"}}})
	p.Steps = append(p.Steps, Step{ID: "joined", Op: "join", Input: "nginx", Other: "state", Key: "pid", OtherKey: "pid", MaxSkewMS: 1000}, Step{ID: "limited", Op: "limit", Input: "nginx", Limit: 1})
	p.Outputs = append(p.Outputs, Output{ID: "joined", Kind: "table", Input: "joined"}, Output{ID: "limited", Kind: "table", Input: "limited"})
	other := c["ps"]
	other.Stdout = "pid=1823\nstate=running"
	c["other"] = other
	d, err := (&Engine{}).Evaluate(context.Background(), p, s, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tables["joined"].Rows) != 1 || *d.Tables["limited"].OmittedRows != 1 || *d.Tables["limited"].TotalRows != 2 {
		t.Fatalf("bad tables %+v", d.Tables)
	}
	other.FinishedAt = other.FinishedAt.Add(2 * time.Second)
	c["other"] = other
	d, err = (&Engine{}).Evaluate(context.Background(), p, s, c, 1)
	if err != nil || d.Tables["joined"].Rows != nil {
		t.Fatal("skew accepted", err)
	}
}
func TestDocumentExampleRemainsExecutable(t *testing.T) {
	raw, err := os.ReadFile("../../docs/monitor-data-pipeline-v2.md")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "```json\n")
	if len(parts) < 2 {
		t.Fatal("missing example")
	}
	var p Plan
	if err = json.Unmarshal([]byte(strings.Split(parts[1], "```")[0]), &p); err != nil {
		t.Fatal(err)
	}
	s, _, c := processFixture()
	d, err := (&Engine{}).Evaluate(context.Background(), p, s, c, 1)
	if err != nil || d.Metrics["rssMiB"].Value != float64(30) {
		t.Fatalf("doc example: %+v %v", d, err)
	}
}

func TestServiceInstanceChangeBreaksRateEvenWhenCounterIncreases(t *testing.T) {
	sources := []Source{{ID: "sql", Command: "status", Required: true, TimeoutMS: 1000}}
	p := Plan{Parsers: []Parser{{ID: "rows", SourceID: "sql", Kind: "kv", Delimiter: "=", Columns: []Column{{ID: "count", Type: "number"}, {ID: "instance", Type: "string"}}}}, Steps: []Step{{ID: "count", Op: "first", Input: "rows", Field: "count"}, {ID: "instance", Op: "first", Input: "rows", Field: "instance"}, {ID: "rate", Op: "rate", Input: "count", Other: "instance", Semantics: "counter"}}, Outputs: []Output{{ID: "rate", Kind: "metric", Input: "rate", Unit: "/s", History: true}}}
	engine := &Engine{}
	now := time.Now()
	zero := 0
	for i, instance := range []string{"old", "old", "new", "new"} {
		raw := fmt.Sprintf("count=%d\ninstance=%s", 100+i*50, instance)
		captures := map[string]Capture{"sql": {Stdout: raw, ExitCode: &zero, Complete: true, Status: "ok", FinishedAt: now.Add(time.Duration(i) * 5 * time.Second)}}
		d, err := engine.Evaluate(context.Background(), p, sources, captures, 1)
		if err != nil {
			t.Fatal(err)
		}
		if (i == 0 || i == 2) && d.Metrics["rate"].Value != nil {
			t.Fatal("restart generated a false rate")
		}
		if (i == 1 || i == 3) && d.Metrics["rate"].Value != float64(10) {
			t.Fatal("rate failed to recover")
		}
	}
}
