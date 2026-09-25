//go:build desktop

package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"

	"git.example.com/ai/wrd-ai-cli/frontend"
	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func main() {
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	service := desktop.New(catalog.Default())
	err = wails.Run(&options.App{Title: "WRD AI SSH", Width: 1180, Height: 760, MinWidth: 940, MinHeight: 650,
		AssetServer: &assetserver.Options{Assets: assets}, BackgroundColour: options.NewRGBA(0, 0, 0, 0),
		Menu: desktop.ApplicationMenu(service),
		Mac: &mac.Options{TitleBar: mac.TitleBarHiddenInset(), WebviewIsTransparent: true, WindowIsTranslucent: true,
			DisableEscapeExitsFullscreen: true, About: &mac.AboutInfo{Title: "WRD AI SSH", Message: "SSH、系统连接与本机凭据管理"}},
		OnStartup: func(ctx context.Context) { desktop.Startup(service, ctx) }, OnShutdown: func(context.Context) { desktop.Shutdown(service) }, Bind: []interface{}{service},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
