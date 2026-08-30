//go:build windows

package autostart

import (
	"fmt"
	"path/filepath"
	"time"

	"haovpn/internal/brand"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func serviceStatus() (installed, running bool, detail string, err error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, false, "", fmt.Errorf("连接服务管理器失败: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return false, false, "服务未安装", nil
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return true, false, "已安装（状态未知）", err
	}
	running = st.State == svc.Running
	if running {
		return true, true, "服务已安装且正在运行（无托盘；查看请再开 GUI 接管）", nil
	}
	return true, false, "服务已安装（未运行）", nil
}

func serviceEnable(exe string, startNow bool) error {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("解析 exe: %w", err)
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败（须管理员）: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(brand.WinServiceName)
	if err == nil {
		// 已存在：确保自动启动
		cfg, errCfg := s.Config()
		if errCfg == nil && cfg.StartType != mgr.StartAutomatic {
			cfg.StartType = mgr.StartAutomatic
			_ = s.UpdateConfig(cfg)
		}
		if startNow {
			_ = s.Start()
		}
		s.Close()
		return nil
	}

	cfg := mgr.Config{
		DisplayName: brand.WinServiceDisplay,
		Description: "工控现场 VPN 客户端，开机自动连接（无托盘）",
		StartType:   mgr.StartAutomatic,
	}
	s, err = m.CreateService(brand.WinServiceName, abs, cfg, "service")
	if err != nil {
		return fmt.Errorf("安装服务失败: %w", err)
	}
	defer s.Close()
	if startNow {
		if err := s.Start(); err != nil {
			return fmt.Errorf("服务已安装但启动失败: %w", err)
		}
	}
	return nil
}

func serviceDisable() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败（须管理员）: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return nil // 未安装视为已禁用
	}
	defer s.Close()
	_, _ = s.Control(svc.Stop)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil || st.State == svc.Stopped {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := s.Delete(); err != nil {
		return fmt.Errorf("卸载服务失败: %w", err)
	}
	return nil
}

func serviceStopAndWait(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return nil // 未安装
	}
	defer s.Close()
	st, err := s.Query()
	if err == nil && st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		// 可能已在停
		_ = err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil {
			return nil
		}
		if st.State == svc.Stopped {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("等待服务停止超时")
}
