//go:build desktop

// Native smoke test: the production Worker/WASM must load over Wails' own scheme.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"testing/fstest"
	"time"

	"git.example.com/ai/wrd-ai-cli/frontend"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type Smoke struct {
	ctx    context.Context
	once   sync.Once
	passed chan bool
}

func (s *Smoke) Report(ok bool, detail string) {
	s.once.Do(func() {
		fmt.Printf("native-widget-smoke passed=%v: %s\n", ok, detail)
		s.passed <- ok
		wailsruntime.Quit(s.ctx)
	})
}

type assets struct {
	base  fs.FS
	index fstest.MapFS
}

func (a assets) Open(name string) (fs.File, error) {
	if name == "index.html" {
		return a.index.Open(name)
	}
	return a.base.Open(name)
}

func main() {
	base, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		panic(err)
	}
	workers, err := fs.Glob(base, "assets/worker-*.js")
	if err != nil || len(workers) != 1 {
		panic("build frontend before native smoke test")
	}
	var fixture struct {
		Artifact widgets.Artifact `json:"artifact"`
		Sample   widgets.Data     `json:"sample"`
	}
	raw, err := os.ReadFile(".cache/widget-v2-fixture.json")
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		panic(err)
	}
	a := fixture.Artifact
	sample, _ := json.Marshal(fixture.Sample)
	init, _ := json.Marshal(map[string]any{"kind": "init", "sequence": 1, "bundle": a.Bundle})
	worker, _ := json.Marshal("/" + workers[0])
	html := `<!doctype html><html><head><meta charset="utf-8"></head><body>Testing native widget runtime<script>
window.addEventListener('DOMContentLoaded',()=>{
const report=(ok,detail)=>window.go.main.Smoke.Report(ok,String(detail));
try {
 const worker=new Worker(` + string(worker) + `,{type:'module'});
 worker.onerror=e=>report(false,'Worker: '+e.message);
 worker.onmessage=e=>{const m=e.data;if(m.error){report(false,m.error);return}if(m.ready){worker.postMessage({kind:'render',sequence:2,data:` + string(sample) + `})}else{report(m.tree?.[0]?.children?.[1]?.props?.value===2&&m.tree?.[0]?.children?.[2]?.props?.value===30,location.href+' rendered '+JSON.stringify(m.tree));worker.terminate()}};
 worker.postMessage(` + string(init) + `);
}catch(e){report(false,e.stack||e)}
});</script></body></html>`
	s := &Smoke{passed: make(chan bool, 1)}
	err = wails.Run(&options.App{Title: "Widget native smoke", Width: 420, Height: 160, StartHidden: true,
		AssetServer: &assetserver.Options{Assets: assets{base, fstest.MapFS{"index.html": {Data: []byte(html)}}}},
		Bind:        []interface{}{s}, OnStartup: func(ctx context.Context) {
			s.ctx = ctx
			go func() { time.Sleep(20 * time.Second); s.Report(false, "native timeout") }()
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	select {
	case ok := <-s.passed:
		if !ok {
			os.Exit(1)
		}
	default:
		os.Exit(1)
	}
}
