//go:build windows

package main

import (
	"os"

	"haovpn/internal/clientapp"
)

// runServiceCommand 将 Windows 服务子命令分派到 clientapp。
func runServiceCommand() bool {
	return clientapp.RunServiceCommand(os.Args)
}
