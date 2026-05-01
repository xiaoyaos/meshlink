//go:build windows

package utils

import (
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsAdmin 检查当前进程是否以管理员身份运行
// 通过尝试打开需要管理员权限的设备来判断
func IsAdmin() bool {
	f, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	if err == nil {
		f.Close()
		return true
	}
	return false
}

// SelfElevate 通过 ShellExecuteEx 触发 Windows UAC 弹窗，以管理员权限重启自身
func SelfElevate() error {
	verb := "runas"
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	// SHELLEXECUTEINFO 结构体
	type shellExecuteInfo struct {
		cbSize         uint32
		fMask          uint32
		hwnd           uintptr
		lpVerb         *uint16
		lpFile         *uint16
		lpParameters   *uint16
		lpDirectory    *uint16
		nShow          int32
		hInstApp       uintptr
		lpIDList       uintptr
		lpClass        *uint16
		hkeyClass      uintptr
		dwHotKey       uint32
		hIconOrMonitor uintptr
		hProcess       uintptr
	}

	info := shellExecuteInfo{
		fMask:        0x00000040, // SEE_MASK_NOCLOSEPROCESS
		lpVerb:       verbPtr,
		lpFile:       exePtr,
		lpParameters: argPtr,
		lpDirectory:  cwdPtr,
		nShow:        1, // SW_NORMAL
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	shellExecuteEx := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	ret, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return err
	}

	// 退出原来的非提权进程
	os.Exit(0)
	return nil
}
