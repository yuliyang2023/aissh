package widgets

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

//go:embed sdk.js
var SDK string

// Compile never evaluates the generated module or resolves user imports on disk.
func Compile(view View) (Artifact, error) {
	if err := view.Validate(); err != nil {
		return Artifact{}, err
	}
	result := api.Build(api.BuildOptions{
		Stdin:  &api.StdinOptions{Contents: "import widget from 'widget-source'; import {__init,__render,__event} from '@wrd/monitor-ui'; __init(widget); export {__render,__event};", Sourcefile: "entry.js", Loader: api.LoaderTSX},
		Bundle: true, Write: false, Format: api.FormatIIFE, GlobalName: "__widgetModule", Target: api.ES2020,
		JSX: api.JSXAutomatic, JSXImportSource: "@wrd/monitor-ui", Platform: api.PlatformNeutral, LogLevel: api.LogLevelSilent,
		Plugins: []api.Plugin{{Name: "widget-modules", Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				if args.Path == "widget-source" && args.Importer == "" {
					return api.OnResolveResult{Path: "source", Namespace: "widget"}, nil
				}
				if args.Path == "widget-source" && args.Importer == "entry.js" {
					return api.OnResolveResult{Path: "source", Namespace: "widget"}, nil
				}
				if args.Kind == api.ResolveJSDynamicImport || (args.Path != "@wrd/monitor-ui" && args.Path != "@wrd/monitor-ui/jsx-runtime") {
					return api.OnResolveResult{}, fmt.Errorf("组件只能导入 @wrd/monitor-ui：%s", args.Path)
				}
				return api.OnResolveResult{Path: "sdk", Namespace: "widget"}, nil
			})
			build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "widget"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				if args.Path == "source" {
					return api.OnLoadResult{Contents: &view.Source, Loader: api.LoaderTSX}, nil
				}
				return api.OnLoadResult{Contents: &SDK, Loader: api.LoaderJS}, nil
			})
		}}},
	})
	if len(result.Errors) > 0 {
		return Artifact{}, errors.New(strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage, Color: false}), "\n"))
	}
	if len(result.OutputFiles) != 1 || len(result.OutputFiles[0].Contents) > MaxBundle {
		return Artifact{}, errors.New("组件编译结果超过限制")
	}
	a := Artifact{View: view, Bundle: string(result.OutputFiles[0].Contents)}
	a.Hash = Hash(view, a.Bundle)
	return a, nil
}

// RunCompiler is a private, bounded stdin/stdout helper; no secrets are inherited.
func RunCompiler(in io.Reader, out io.Writer) error {
	b, err := io.ReadAll(io.LimitReader(in, MaxSource*2+1))
	if err != nil {
		return err
	}
	if len(b) > MaxSource*2 {
		return errors.New("编译请求过大")
	}
	var v View
	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}
	a, err := Compile(v)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(a)
}

var compilerSlots = make(chan struct{}, 2)

func CompileIsolated(ctx context.Context, view View) (Artifact, error) {
	executable, err := os.Executable()
	if err != nil {
		return Artifact{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case compilerSlots <- struct{}{}:
		defer func() { <-compilerSlots }()
	case <-ctx.Done():
		return Artifact{}, ctx.Err()
	}
	input, _ := json.Marshal(view)
	cmd := exec.CommandContext(ctx, executable, "--widget-compile")
	cmd.Stdin = bytes.NewReader(input)
	cmd.Dir = os.TempDir()
	cmd.Env = []string{}
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	var output, diagnostic limitedBuffer
	output.limit = MaxBundle * 3
	diagnostic.limit = 8192
	cmd.Stdout = &output
	cmd.Stderr = &diagnostic
	cmd.WaitDelay = time.Second
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Artifact{}, errors.New("组件编译超时")
		}
		return Artifact{}, fmt.Errorf("组件编译失败：%s", diagnostic.String())
	}
	var a Artifact
	if err = json.Unmarshal(output.Bytes(), &a); err != nil {
		return a, err
	}
	return a, a.Validate()
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}
