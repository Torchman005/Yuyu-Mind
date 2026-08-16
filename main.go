package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/yuyu-mind/backend/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	application := app.New()

	err := wails.Run(&options.App{
		Title:  "Yuyu Mind",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
		Bind: []interface{}{
			application,
		},
		// 桌宠模式需要透明的原生窗口，避免 WebView2 底层的黑色背景透出。
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.None,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
