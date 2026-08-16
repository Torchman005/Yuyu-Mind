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
		Title:     "Yuyu Mind",
		Width:     1200,
		Height:    800,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  application.Startup,
		OnShutdown: application.Shutdown,
		Bind: []interface{}{
			application,
		},
		// 无边框 + 桌宠模式透明原生窗口：
		// - Frameless 去掉系统标题栏/边框，完整模式用前端自定义 header（拖拽区 + 最小化/关闭按钮）。
		// - WebviewIsTransparent + WindowIsTranslucent + BackdropType None 让桌宠模式背景全透明。
		// - DisableFramelessWindowDecorations 去掉 Aero 阴影/圆角，避免透明桌宠四周出现矩形阴影。
		Windows: &windows.Options{
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               true,
			BackdropType:                      windows.None,
			DisableFramelessWindowDecorations: true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
