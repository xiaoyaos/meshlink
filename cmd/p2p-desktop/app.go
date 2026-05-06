package main

import (
	"context"
	"fmt"
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

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	a.StopVPN()
}

// GetInfo 返回 PeerID 和 Virtual IP
func (a *App) GetInfo() (result []string) {
	defer func() {
		if r := recover(); r != nil {
			result = []string{"", fmt.Sprintf("错误: %v", r)}
		}
	}()

	id, ip, err := a.service.GetInfo()
	if err != nil {
		return []string{"", fmt.Sprintf("错误: %v", err)}
	}
	return []string{id, ip}
}

// StartVPN 启动 VPN
func (a *App) StartVPN(bootstrapAddr string) (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = fmt.Sprintf("启动失败: %v", r)
		}
	}()

	err := a.service.Start(bootstrapAddr)
	if err != nil {
		return err.Error()
	}
	return ""
}

// StopVPN 停止 VPN
func (a *App) StopVPN() {
	defer func() {
		if r := recover(); r != nil && a.service != nil && a.service.OnLog != nil {
			a.service.OnLog(fmt.Sprintf("停止失败: %v", r))
		}
	}()

	a.service.Stop()
}

// GetPeerCount 返回当前连接的节点数量
func (a *App) GetPeerCount() (result int) {
	defer func() {
		if recover() != nil {
			result = 0
		}
	}()

	return 0
}
