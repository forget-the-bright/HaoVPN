//go:build windows

package autostart

import (
	"fmt"
	"time"

	"haovpn/internal/brand"
	"haovpn/internal/fileutil"
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// winServiceDescription SCM 服务描述（CLI / GUI 共用唯一文案）。
const winServiceDescription = "工控现场 VPN 客户端，开机自动连接（无托盘）"

// connectSCM 连接本地服务控制管理器；失败时提示须管理员。
func connectSCM() (*mgr.Mgr, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("连接服务管理器失败（须管理员）: %w", err)
	}
	return m, nil
}

func serviceStatus() (installed, running bool, detail string, err error) {
	m, err := connectSCM()
	if err != nil {
		return false, false, "", err
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

// serviceInstall 创建或修好已有服务（不 Start）。
func serviceInstall(exe string) error {
	abs, _, err := fileutil.AbsPair(exe, "")
	if err != nil {
		return err
	}
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(brand.WinServiceName)
	if err == nil {
		defer s.Close()
		if err := ensureServiceConfig(s); err != nil {
			return err
		}
		logger.Info("autostart service already installed name=%s bin=%s", brand.WinServiceName, abs)
		return nil
	}

	cfg := mgr.Config{
		DisplayName: brand.WinServiceDisplay,
		Description: winServiceDescription,
		StartType:   mgr.StartAutomatic,
	}
	s, err = m.CreateService(brand.WinServiceName, abs, cfg, "service")
	if err != nil {
		return fmt.Errorf("安装服务失败: %w", err)
	}
	s.Close()
	logger.Info("autostart service installed name=%s bin=%s", brand.WinServiceName, abs)
	return nil
}

// ensureServiceConfig 已存在服务：自动启动 + 显示名/描述对齐。
//
// 不改 BinaryPathName：SCM 存的是「exe + 参数 service」带引号形式，
// 与纯 Abs 路径字符串不相等；强行改写易打断正在跑的服务。换二进制请先卸载再装。
func ensureServiceConfig(s *mgr.Service) error {
	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("读取服务配置失败: %w", err)
	}
	changed := false
	if cfg.StartType != mgr.StartAutomatic {
		cfg.StartType = mgr.StartAutomatic
		changed = true
	}
	if cfg.Description != winServiceDescription {
		cfg.Description = winServiceDescription
		changed = true
	}
	if cfg.DisplayName != brand.WinServiceDisplay {
		cfg.DisplayName = brand.WinServiceDisplay
		changed = true
	}
	if !changed {
		return nil
	}
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("更新服务配置失败: %w", err)
	}
	return nil
}

func serviceStart() error {
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return fmt.Errorf("服务未安装: %w", err)
	}
	defer s.Close()
	st, err := s.Query()
	if err == nil && st.State == svc.Running {
		return nil
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}
	logger.Info("autostart service started name=%s", brand.WinServiceName)
	return nil
}

func serviceEnable(exe string, startNow bool) error {
	if err := serviceInstall(exe); err != nil {
		return err
	}
	if !startNow {
		return nil
	}
	if err := serviceStart(); err != nil {
		return err
	}
	return nil
}

func serviceDisable() error {
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return nil // 未安装视为已禁用
	}
	defer s.Close()
	if err := waitServiceStopped(s, DefaultServiceStopTimeout); err != nil {
		logger.Warn("autostart service stop before uninstall: %v", err)
		// 仍尝试 Delete：部分环境停不完全但可删
	}
	if err := s.Delete(); err != nil {
		return fmt.Errorf("卸载服务失败: %w", err)
	}
	logger.Info("autostart service uninstalled name=%s", brand.WinServiceName)
	return nil
}

func serviceStopAndWait(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultServiceStopTimeout
	}
	m, err := connectSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(brand.WinServiceName)
	if err != nil {
		return nil // 未安装
	}
	defer s.Close()
	return waitServiceStopped(s, timeout)
}

// waitServiceStopped 发 Stop 并轮询直至 Stopped 或超时。
func waitServiceStopped(s *mgr.Service, timeout time.Duration) error {
	st, err := s.Query()
	if err == nil && st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		// 可能已在停止中；继续轮询
		logger.Debug("autostart service Control(Stop): %v", err)
	}
	ok := safeutil.PollUntil(time.Now().Add(timeout), 100*time.Millisecond, nil, func() bool {
		st, err := s.Query()
		if err != nil {
			// 查询失败（服务可能已删）视为已停
			return true
		}
		return st.State == svc.Stopped
	})
	if ok {
		return nil
	}
	return fmt.Errorf("等待服务停止超时（%s）", timeout)
}
