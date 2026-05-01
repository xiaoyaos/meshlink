//go:build windows

package tun

import (
	_ "embed"
	"os"
	"path/filepath"
)

// wintun.dll is embedded directly into the binary for zero-dependency deployment
//
//go:embed wintun.dll
var wintunDLL []byte

// EnsureWintun 将嵌入的 wintun.dll 释放到可执行文件同级目录下
// 这样用户无需手动安装任何驱动，双击即可运行
func EnsureWintun() error {
	if len(wintunDLL) == 0 {
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	dllPath := filepath.Join(filepath.Dir(execPath), "wintun.dll")

	// 如果已经存在则跳过（避免每次启动都写）
	if _, err := os.Stat(dllPath); err == nil {
		return nil
	}

	return os.WriteFile(dllPath, wintunDLL, 0644)
}
