//go:build !windows

package wintundll

// Ensure 非 Windows 平台无操作。
func Ensure() error { return nil }
