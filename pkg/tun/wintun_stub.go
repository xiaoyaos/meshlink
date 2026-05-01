//go:build !windows
package tun

// EnsureWintun 在非 Windows 平台上不做任何操作
func EnsureWintun() error {
	return nil
}
