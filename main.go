package main

import (
	"context"
	"embed"

	"spark-todo/internal/todo"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets 将前端构建产物（`frontend/dist`）打包进 Go 二进制。
//
// Wails 会通过内置的 AssetServer 提供这些静态资源，使应用在发布时无需额外携带前端文件目录。
//
//go:embed all:frontend/dist
var assets embed.FS

// readConciseModeSetting 在应用启动前读取 conciseMode 设置。
//
// 用于决定窗口是否使用 Frameless 模式（无边框）。
// 如果读取失败，返回默认值 false（有边框）。
func readConciseModeSetting() bool {
	dbPath, err := todo.DefaultDBPath("Spark-Todo")
	if err != nil {
		return false
	}

	store, err := todo.Open(dbPath)
	if err != nil {
		return false
	}

	settings, err := store.GetSettings(context.Background())
	// 先获取设置，再关闭数据库连接
	_ = store.Close()

	if err != nil {
		return false
	}

	return settings.ConciseMode
}

func main() {
	// NewApp 创建应用的后端实例：
	// - 持有运行时上下文（用于调用 Wails runtime API）
	// - 持有 Store（SQLite 持久化），并对外暴露给前端调用的方法（Bind）
	app := NewApp()

	// 读取 conciseMode 设置以决定窗口是否使用无边框模式
	frameless := readConciseModeSetting()

	// wails.Run 启动 GUI 事件循环，并将后端对象绑定到前端 JS：
	// - Window 配置：尺寸偏"小挂件"，适合常驻桌面角落
	// - Frameless：根据用户的 conciseMode 设置决定是否显示窗口边框
	// - AlwaysOnTop 初始不强制置顶：由 startup 读取持久化设置后再决定是否置顶
	// - AssetServer：使用上方 embed 的前端资源
	err := wails.Run(&options.App{
		Title:       "Spark-Todo",
		Width:       450,
		Height:      300,
		MinWidth:    200,
		MinHeight:   200,
		Frameless:   frameless,
		AlwaysOnTop: false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 247, G: 249, B: 251, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
