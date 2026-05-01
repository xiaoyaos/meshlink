//go:build !windows

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// IsAdmin 检查当前进程是否具有 Root 权限
func IsAdmin() bool {
	return os.Geteuid() == 0
}

// SelfElevate 在 macOS/Linux 上尝试提升权限并重新启动进程
func SelfElevate() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		// macOS: 使用 osascript 弹出密码框提权
		script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, exe)
		cmd := exec.Command("osascript", "-e", script)
		return cmd.Start()
	case "linux":
		// Linux: 尝试使用 pkexec
		cmd := exec.Command("pkexec", exe)
		return cmd.Start()
	default:
		return fmt.Errorf("self-elevation not supported on %s", runtime.GOOS)
	}
}
