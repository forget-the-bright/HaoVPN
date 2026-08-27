//go:build !windows

package main

// runServiceCommand 非 Windows 平台无服务子命令。
func runServiceCommand() bool { return false }
