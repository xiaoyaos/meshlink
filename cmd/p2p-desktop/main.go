package main

import (
	"embed"
	"p2p/pkg/tun"
	"p2p/pkg/utils"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 0. Windows 驱动初始化
	if runtime.GOOS == "windows" {
		_ = tun.EnsureWintun()
	}

	// 1. 权限检测与自动提权 (打包成正式应用时需要)
	if !utils.IsAdmin() {
		err := utils.SelfElevate()
		if err != nil {
			println("Error: Admin privileges required!", err.Error())
		}
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "P2P Mesh Network",
		Width:  600,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
