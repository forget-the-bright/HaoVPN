package netstack

import "haovpn/internal/winnet"

// WindowsOptions 客户端 Windows 网卡加速开关（由 client.yaml windows 段注入）。
//
// 本类型是编排层对外契约；底层仍写入 winnet.Options。
// 为何不直接让 clientapp import winnet：保持 clientapp → netstack → winnet 单向依赖。
type WindowsOptions struct {
	// UseIPHelper 为 true 时读/写优先 IP Helper，失败再回退 netsh/route。
	UseIPHelper bool
}

// ConfigureWindows 在客户端引擎启动时注入 Windows 加速开关；可重复调用。
//
// 参数：o — 通常来自 config.ClientWindowsSection.UseIPHelperEnabled()。
// 关联：clientapp.NewEngine；winnet.Configure。
func ConfigureWindows(o WindowsOptions) {
	winnet.Configure(winnet.Options{UseIPHelper: o.UseIPHelper})
}

// ShutdownWindows 进程退出前 Windows 网卡子系统挂点（当前为空操作，见 winnet.Shutdown）。
//
// 关联：clientapp.Engine.Stop / GUI 退出；禁止再引入常驻 PowerShell 主机。
func ShutdownWindows() {
	winnet.Shutdown()
}

// HasICSResidue 探测 TUN 是否仍残留 ICS（如 192.168.137.x）地址。
//
// 参数：configName — TUN/yaml 配置名（如 haovpn0）。
// 返回：true 表示有残留，空 local_lans 路径才值得跑慢清理。
// 关联：clientapp via_exit cleanupTUNAfterViaDisabled。
func HasICSResidue(configName string) bool {
	return winnet.HasICSResidue(configName)
}

// CleanupICSResidue 一次清理 ICS 共享并删除 137 地址，保留 vpnIP。
//
// 参数：configName — TUN 名；vpnIP — 须保留的 VPN 地址。
func CleanupICSResidue(configName, vpnIP string) error {
	return winnet.CleanupICSResidue(configName, vpnIP)
}

// RemoveICSAddressesKeepVPN 仅删除 ICS 残留地址，不关全机共享（hadVia 快路径）。
func RemoveICSAddressesKeepVPN(configName, vpnIP string) error {
	return winnet.RemoveICSAddressesKeepVPN(configName, vpnIP)
}

// DisableAllICS 关闭本机全部 ICS 共享（慢路径；Teardown 回退用）。
func DisableAllICS() {
	winnet.DisableAllICS()
}

// DisableICSPair 靶向关闭一对 public/private ICS 共享。
//
// 参数：public / private — 适配器名；空则无操作。
func DisableICSPair(public, private string) {
	winnet.DisableICSPair(public, private)
}
