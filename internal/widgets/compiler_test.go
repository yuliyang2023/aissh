package widgets

import (
	"strings"
	"testing"
)

func TestCompilerBoundary(t *testing.T) {
	good := View{Kind: "custom", SDKVersion: 1, StateVersion: 1, Source: `import {defineWidget, MetricCard} from '@wrd/monitor-ui'; export default defineWidget({initialState:{},render({data}){return <MetricCard label="test" value={data?.n ?? null}/>}})`}
	a, err := Compile(good)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Validate(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Bundle, "__render") {
		t.Fatal("runtime missing")
	}
	for _, source := range []string{`import x from '/etc/passwd'; export default x`, `import x from 'node:fs'; export default x`, `export default import('https://example.com/x.js')`, `import x from '../secret'; export default x`, `export default <div>`} {
		v := good
		v.Source = source
		if _, err := Compile(v); err == nil {
			t.Fatalf("accepted %s", source)
		}
	}
}
func TestStructuredData(t *testing.T) {
	c := Contract{Format: "json", Fields: []Field{{Name: "n", Type: "number", History: true}, {Name: "status", Type: "string", Nullable: true}}}
	d, issues, err := Parse(c, `{"n":0}`)
	if err != nil || d["n"] != float64(0) || len(issues) != 1 {
		t.Fatalf("%v %v %v", d, issues, err)
	}
	for _, raw := range []string{`{"n":1,"n":2}`, `{"n":"0"}`, `{"n":1} {}`, `{"n":1e999}`, `{"n":9007199254740993}`, `{"status":"up"}`, `{"n":0,"extra":1}`, `{"__proto__":{},"n":1}`} {
		if _, _, err := Parse(c, raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	c.Format = "kv"
	if _, _, err := Parse(c, "n=1\nstatus=up"); err != nil {
		t.Fatal(err)
	}
}
