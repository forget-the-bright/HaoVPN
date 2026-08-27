//go:build !windows

package clientapp

// RunServiceCommand 非 Windows 平台无服务子命令。
func RunServiceCommand(args []string) bool { return false }
