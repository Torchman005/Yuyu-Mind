package main

import (
	"github.com/yuyu-mind/backend/internal/app"
)

// App 是 Wails 绑定层包装，实际逻辑委托给 internal/app。
// Wails 要求绑定结构体位于 main package。
type App = app.App
