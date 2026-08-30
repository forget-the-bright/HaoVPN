package autostart

import "time"

// ServiceStatus 查询 HaoVPN 客户端 Windows 服务是否已安装/运行。
func ServiceStatus() (installed, running bool, detail string, err error) {
	return serviceStatus()
}

// ServiceEnable 用指定 exe 安装并设为自动启动，可选立即 Start。
//
// exe 通常为 haovpn-client-gui.exe 或 haovpn-client.exe（均须支持 args「service」）。
func ServiceEnable(exe string, startNow bool) error {
	return serviceEnable(exe, startNow)
}

// ServiceDisable 停止并卸载服务。
func ServiceDisable() error {
	return serviceDisable()
}

// ServiceStopAndWait 停止服务并等待 Stopped（接管 GUI 前调用）。
func ServiceStopAndWait(timeout time.Duration) error {
	return serviceStopAndWait(timeout)
}
