package main

import (
	"context"
	"os"
	"path/filepath"

	"p2p/pkg/app"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx     context.Context
	service *app.VPNService
}

// configDir 返回跨平台稳定的配置目录路径
func configDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		// 兜底：使用用户主目录
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, "P2PMesh")
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		service: app.NewVPNService(configDir(), "4001"),
	}
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 绑定日志回调
	a.service.OnLog = func(msg string) {
		runtime.EventsEmit(a.ctx, "vpn_log", msg)
	}

	// 绑定状态回调
	a.service.OnState = func(st app.State) {
		runtime.EventsEmit(a.ctx, "vpn_state", int(st))
	}
}

// GetInfo 返回 PeerID 和 Virtual IP
func (a *App) GetInfo() []string {
	id, ip := a.service.GetInfo()
	return []string{id, ip}
}

// StartVPN 启动 VPN
func (a *App) StartVPN(bootstrapAddr string) string {
	err := a.service.Start(bootstrapAddr)
	if err != nil {
		return err.Error()
	}
	return ""
}

// StopVPN 停止 VPN
func (a *App) StopVPN() {
	a.service.Stop()
}

// GetPeerCount 返回当前连接的节点数量
func (a *App) GetPeerCount() int {
	return 0
}
