package autostart

import "time"

// DefaultServiceStopTimeout 停止 Windows 服务并等待 Stopped 的默认超时。
// CLI --service stop、GUI 接管、ServiceDisable 卸载前共用，避免各处各写 20s。
const DefaultServiceStopTimeout = 20 * time.Second

// ServiceStatus 查询客户端服务是否已安装/运行。
//
// Windows：查 SCM（brand.WinServiceName）。
// 其他平台：见各平台 service_*.go（systemd/launchd 或诚实提示）。
func ServiceStatus() (installed, running bool, detail string, err error) {
	return serviceStatus()
}

// ServiceInstall 用指定 exe 安装服务并设为自动启动（不立即 Start）。
//
// exe 须为绝对或可 Abs 的路径；服务启动参数固定为「service」（由 SCM 传入 argv）。
// 若服务已存在：确保 StartType=Automatic，并尽量更新 BinaryPathName 为当前 exe。
func ServiceInstall(exe string) error {
	return serviceInstall(exe)
}

// ServiceStart 启动已安装的客户端服务。
func ServiceStart() error {
	return serviceStart()
}

// ServiceStop 请求停止服务并等待进入 Stopped（超时用 DefaultServiceStopTimeout）。
func ServiceStop() error {
	return serviceStopAndWait(DefaultServiceStopTimeout)
}

// ServiceUninstall 停止（若在跑）并删除服务；未安装视为成功。
func ServiceUninstall() error {
	return serviceDisable()
}

// ServiceEnable 安装（或修好已有服务）并可选立即 Start。
//
// exe 通常为 haovpn-client-gui.exe 或 haovpn-client.exe（均须支持 args「service」）。
// 托盘「开机无窗口服务」走此入口；失败必须返回 error（禁止伪成功）。
func ServiceEnable(exe string, startNow bool) error {
	return serviceEnable(exe, startNow)
}

// ServiceDisable 停止并卸载服务（同 ServiceUninstall）。
func ServiceDisable() error {
	return serviceDisable()
}

// ServiceStopAndWait 停止服务并等待 Stopped（GUI 接管前调用）。
// timeout<=0 时使用 DefaultServiceStopTimeout。
func ServiceStopAndWait(timeout time.Duration) error {
	return serviceStopAndWait(timeout)
}
