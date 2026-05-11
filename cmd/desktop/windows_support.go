//go:build windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func ensureWindowsAdmin() error {
	admin, err := isWindowsAdmin()
	if err != nil || admin {
		return err
	}
	return relaunchWindowsElevated()
}

func isWindowsAdmin() (bool, error) {
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}
	return windows.GetCurrentProcessToken().IsMember(adminSID)
}

func relaunchWindowsElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	args, err := syscall.UTF16PtrFromString(windows.ComposeCommandLine(os.Args[1:]))
	if err != nil {
		return err
	}
	dir, err := syscall.UTF16PtrFromString(cwd)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, file, args, dir, windows.SW_NORMAL); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

func hideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func splitWindowsCommandLine(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	return windows.DecomposeCommandLine(line)
}
