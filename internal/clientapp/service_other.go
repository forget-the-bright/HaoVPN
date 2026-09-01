//go:build !windows

package clientapp

import (
	"fmt"
	"os"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/singleinstance"
)

// RunServiceCommand 非 Windows：支持无界面入口「service」（供 systemd/launchd ExecStart）。
//
// 与 Windows SCM 一致：argv[1]=="service" 时跑 VPN 主循环（无 Fyne）。
// 不提供 install/start/stop CLI（改用 systemctl/launchctl 或 autostart API）。
func RunServiceCommand(args []string) bool {
	if len(args) < 2 || args[1] != "service" {
		return false
	}
	logger.Info("clientapp unix headless service entry")
	lock, err := singleinstance.AcquireClient()
	if err != nil {
		fmt.Println(singleinstance.AlreadyRunningMessage())
		os.Exit(1)
	}
	defer lock.Release()
	cfgPath := config.ResolveClientConfigPath()
	if err := RunServiceLoop(cfgPath, Credentials{}); err != nil {
		fmt.Println("服务运行失败:", err)
		os.Exit(1)
	}
	return true
}
