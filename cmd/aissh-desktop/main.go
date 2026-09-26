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
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func main() {
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) == 2 && os.Args[1] == "--desktop-pet" {
		err = runPet(assets)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	service := desktop.New(catalog.Default())
	err = wails.Run(&options.App{Title: "WRD AI SSH", Width: 1180, Height: 760, MinWidth: 940, MinHeight: 650,
		AssetServer: &assetserver.Options{Assets: assets}, BackgroundColour: options.NewRGBA(0, 0, 0, 0),
		Menu: desktop.ApplicationMenu(service),
		Mac: &mac.Options{TitleBar: mac.TitleBarHiddenInset(), WebviewIsTransparent: true, WindowIsTranslucent: true,
			DisableEscapeExitsFullscreen: true, About: &mac.AboutInfo{Title: "WRD AI SSH", Message: "SSH、系统连接与本机凭据管理"}},
		OnStartup: func(ctx context.Context) { desktop.Startup(service, ctx) }, OnDomReady: func(context.Context) { desktop.DesktopReady(service) }, OnShutdown: func(context.Context) { desktop.Shutdown(service) }, Bind: []interface{}{service},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runPet(assets fs.FS) error {
	bridge, err := desktop.NewPetBridge()
	if err != nil {
		return err
	}
	return wails.Run(&options.App{
		Title: "监控小猫", Width: 320, Height: 330, DisableResize: true, Frameless: true, AlwaysOnTop: true,
		BackgroundColour: options.NewRGBA(0, 0, 0, 0), AssetServer: &assetserver.Options{Assets: assets},
		Mac:       &mac.Options{WebviewIsTransparent: true, DisableZoom: true},
		Windows:   &windows.Options{WebviewIsTransparent: true, WindowIsTranslucent: true, DisableWindowIcon: true},
		Linux:     &linux.Options{WindowIsTranslucent: true},
		OnStartup: func(ctx context.Context) { desktop.PetStartup(bridge, ctx) },
		OnDomReady: func(ctx context.Context) {
			configurePetWindow()
			if screens, err := wailsruntime.ScreenGetAll(ctx); err == nil {
				for _, screen := range screens {
					if screen.IsPrimary {
						wailsruntime.WindowSetPosition(ctx, max(0, screen.Size.Width-350), max(0, screen.Size.Height-410))
						break
					}
				}
			}
		},
		Bind: []interface{}{bridge},
	})
}
