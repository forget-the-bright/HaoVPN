//go:build windows

package clientapp

import (
	"fmt"
	"os"

	"haovpn/internal/autostart"
	"haovpn/internal/brand"
	"haovpn/internal/config"
	"haovpn/internal/safeutil"
	"haovpn/internal/singleinstance"

	"golang.org/x/sys/windows/svc"
)

// RunServiceCommand 处理 Windows 服务 install/start/stop/uninstall 与 SCM 启动入口。
//
// SCM 安装/启停/卸载全部委托 internal/autostart（与 GUI 托盘共用单一真相源）。
// 本文件只保留：CLI 薄封装 + svc.Run 主循环（VPN 领域在 clientapp）。
//
// 参数 args 通常为 os.Args；已处理时返回 true，main 应直接 return。
func RunServiceCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[1] {
	case "--service":
		if len(args) < 3 {
			fmt.Println("用法: haovpn-client --service install|start|stop|uninstall")
			os.Exit(1)
		}
		switch args[2] {
		case "install":
			cliServiceInstall()
		case "start":
			cliServiceStart()
		case "stop":
			cliServiceStop()
		case "uninstall":
			cliServiceUninstall()
		default:
			fmt.Println("未知子命令:", args[2])
			os.Exit(1)
		}
		return true
	case "service":
		// 由 SCM 启动：运行 VPN 客户端主循环直至收到停止信号
		_ = svc.Run(brand.WinServiceName, &clientService{})
		return true
	}
	return false
}

// clientService 实现 golang.org/x/sys/windows/svc.Handler。
type clientService struct{}

func (m *clientService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	clientLock, err := singleinstance.AcquireClient()
	if err != nil {
		fmt.Println(singleinstance.AlreadyRunningMessage())
		return false, 1
	}
	defer clientLock.Release()

	cfgPath := config.ResolveClientConfigPath()
	errCh := make(chan error, 1)
	safeutil.GoSafe("windows-service-client", func() {
		errCh <- RunCLI(cfgPath, Credentials{})
	})

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-r:
			if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
				changes <- svc.Status{State: svc.StopPending}
				StopCLI()
				if err := <-errCh; err != nil {
					return false, 1
				}
				return false, 0
			}
		case err := <-errCh:
			if err != nil {
				return false, 1
			}
			return false, 0
		}
	}
}

func cliServiceInstall() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("解析本程序路径失败:", err)
		os.Exit(1)
	}
	if err := autostart.ServiceInstall(exe); err != nil {
		fmt.Println("安装失败:", err)
		os.Exit(1)
	}
	fmt.Println("服务已安装，执行 --service start 启动")
}

func cliServiceStart() {
	if err := autostart.ServiceStart(); err != nil {
		fmt.Println("启动失败:", err)
		os.Exit(1)
	}
	fmt.Println("服务已启动")
}

func cliServiceStop() {
	if err := autostart.ServiceStop(); err != nil {
		fmt.Println("停止失败:", err)
		os.Exit(1)
	}
	fmt.Println("服务已停止")
}

func cliServiceUninstall() {
	if err := autostart.ServiceUninstall(); err != nil {
		fmt.Println("卸载失败:", err)
		os.Exit(1)
	}
	fmt.Println("服务已卸载")
}
