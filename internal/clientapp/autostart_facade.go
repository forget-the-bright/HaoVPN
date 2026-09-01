package clientapp

import (
	"os"
	"path/filepath"
	"time"

	"haovpn/internal/autostart"
)

// ResolveClientExecutable 返回当前进程 exe 的绝对路径（GUI/CLI 自启注册共用）。
func ResolveClientExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

// LogonAutostartStatus 登录后计划任务自启是否已启用。
func LogonAutostartStatus() (enabled bool, detail string, err error) {
	return autostart.LogonStatus()
}

// LogonAutostartEnable 注册登录后启动本程序的计划任务。
func LogonAutostartEnable(exe, cfgPath string) error {
	return autostart.LogonEnable(exe, cfgPath)
}

// LogonAutostartDisable 取消登录后计划任务。
func LogonAutostartDisable() error {
	return autostart.LogonDisable()
}

// ServiceAutostartStatus 返回服务是否已安装及运行状态。
func ServiceAutostartStatus() (installed, running bool, detail string, err error) {
	return autostart.ServiceStatus()
}

// ServiceAutostartEnable 安装并可选立即启动 Windows 服务（无托盘）。
//
// startNow 为 false 时仅安装，避免与已连接 GUI 抢单实例锁。
func ServiceAutostartEnable(exe string, startNow bool) error {
	return autostart.ServiceEnable(exe, startNow)
}

// ServiceAutostartDisable 卸载服务开机自启，并 best-effort 清理服务 DPAPI 凭据文件。
func ServiceAutostartDisable() error {
	if err := autostart.ServiceDisable(); err != nil {
		return err
	}
	_ = DeleteServiceCredentials()
	return nil
}

// DefaultServiceStopTimeout GUI 接管前停止服务的默认等待时长（re-export autostart）。
const DefaultServiceStopTimeout = autostart.DefaultServiceStopTimeout

// ServiceStopAndWait 停止 HaoVPN 客户端服务并等待 Stopped（GUI 服务接管用）。
func ServiceStopAndWait(timeout time.Duration) error {
	return autostart.ServiceStopAndWait(timeout)
}
