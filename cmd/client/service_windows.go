//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"haovpn/internal/brand"
	"haovpn/internal/safeutil"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// runServiceCommand 处理 Windows 服务 install/start/stop（v1.0 工程师笔记本开机自连）。
func runServiceCommand() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "--service":
		if len(os.Args) < 3 {
			fmt.Println("用法: haovpn-client --service install|start|stop|uninstall")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "install":
			installService()
		case "start":
			startService()
		case "stop":
			stopService()
		case "uninstall":
			uninstallService()
		default:
			fmt.Println("未知子命令:", os.Args[2])
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

const serviceName = brand.WinServiceName

type clientService struct{}

func (m *clientService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	cfgPath := serviceConfigPath()
	errCh := make(chan error, 1)
	safeutil.GoSafe("windows-service-client", func() {
		errCh <- runClient(cfgPath)
	})

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-r:
			if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
				changes <- svc.Status{State: svc.StopPending}
				stopClient()
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

// serviceConfigPath 返回与可执行文件同目录的 client.yaml（服务 WorkingDirectory 常为 System32）。
func serviceConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "./client.yaml"
	}
	return filepath.Join(filepath.Dir(exe), "client.yaml")
}

func installService() {
	exe, _ := os.Executable()
	m, err := mgr.Connect()
	if err != nil {
		fmt.Println("连接服务管理器失败:", err)
		os.Exit(1)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		fmt.Println("服务已存在")
		return
	}
	cfg := mgr.Config{
		DisplayName: brand.WinServiceDisplay,
		Description: "工控现场 VPN 客户端，开机自动连接",
		StartType:   mgr.StartAutomatic,
	}
	s, err = m.CreateService(serviceName, exe, cfg, "service")
	if err != nil {
		fmt.Println("安装失败:", err)
		os.Exit(1)
	}
	defer s.Close()
	fmt.Println("服务已安装，执行 --service start 启动")
}

func startService()  { controlService("start") }
func stopService()   { controlService("stop") }
func uninstallService() {
	m, _ := mgr.Connect()
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return
	}
	defer s.Close()
	_ = s.Delete()
	fmt.Println("服务已卸载")
}

func controlService(action string) {
	m, err := mgr.Connect()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		fmt.Println("服务未安装")
		os.Exit(1)
	}
	defer s.Close()
	switch action {
	case "start":
		if err := s.Start(); err != nil {
			fmt.Println("启动失败:", err)
			os.Exit(1)
		}
		fmt.Println("服务已启动")
	case "stop":
		_, _ = s.Control(svc.Stop)
		fmt.Println("服务已停止")
	}
}
