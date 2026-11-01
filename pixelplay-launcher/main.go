package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"pixelplay-launcher/internal/studiotest"
	"pixelplay-launcher/internal/webkitperm"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--studio-test" {
		dir := ""
		if len(os.Args) >= 3 {
			dir = os.Args[2]
		}
		if dir == "" {
			fmt.Fprintln(os.Stderr, "uso: pixelplay-launcher --studio-test <directorio-del-proyecto>")
			os.Exit(2)
		}
		if err := studiotest.Run(dir); err != nil {
			fmt.Fprintln(os.Stderr, "studio-test:", err)
			os.Exit(1)
		}
		return
	}

	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "Pixel Play",
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		Frameless:        true,
		CSSDragProperty:  "--wails-draggable",
		CSSDragValue:     "drag",
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 12, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			webkitperm.Install()
			app.Startup(ctx)
		},
		OnDomReady: func(ctx context.Context) {
			webkitperm.Install()
		},
		Bind: []interface{}{app},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		Linux: &linux.Options{
			Icon:             appIcon,
			ProgramName:      "pixelplay-launcher",
			WebviewGpuPolicy: linux.WebviewGpuPolicyAlways,
		},
		Windows: &windows.Options{
			DisableWindowIcon: false,
			Theme:             windows.Dark,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   "Pixel Play",
				Message: "Launcher oficial de Pixel Play",
				Icon:    appIcon,
			},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
