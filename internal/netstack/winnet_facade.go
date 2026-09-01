package netstack

import (
	"context"
	"fmt"

	"haovpn/internal/logger"
	"haovpn/internal/winnet"
)

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
// 说明：进程退出无单独 Shutdown 钩子（曾用于常驻 PS，已删除；禁止再引入）。
func ConfigureWindows(o WindowsOptions) {
	winnet.Configure(winnet.Options{UseIPHelper: o.UseIPHelper})
}

// HasICSResidue 探测 TUN 是否仍残留 ICS（如 192.168.137.x）地址。
//
// 参数：configName — TUN/yaml 配置名（如 haovpn0）。
// 返回：true 表示有残留，空 local_lans 路径才值得跑慢清理。
// 关联：clientapp via_exit cleanupTUNAfterViaDisabled。
func HasICSResidue(configName string) bool {
	return winnet.HasICSResidue(configName)
}

// CleanupICSResidueContext 一次清理 ICS 共享并删除 137 地址，保留 vpnIP；ctx 取消时 Kill PowerShell。
//
// 关联：clientapp via_exit cleanupTUNAfterViaDisabled（空 local_lans 有残留且非 hadVia）。
func CleanupICSResidueContext(ctx context.Context, configName, vpnIP string) error {
	return winnet.CleanupICSResidueContext(ctx, configName, vpnIP)
}

// RemoveICSAddressesKeepVPN 仅删除 ICS 残留地址，不关全机共享（hadVia 快路径）。
func RemoveICSAddressesKeepVPN(configName, vpnIP string) error {
	return winnet.RemoveICSAddressesKeepVPN(configName, vpnIP)
}

// ScrubTUNDefaultRouteFast 快路径：无路由 skip，iphlp 成功即返回，不起 PS。
func ScrubTUNDefaultRouteFast(configName string) {
	idx, err := winnet.InterfaceIndex(configName)
	if err != nil || idx <= 0 {
		logger.Debug("tun_default_route_scrub skip resolve tun=%s err=%v", configName, err)
		return
	}
	if _, e := winnet.DeleteDefaultRouteOnInterface(idx); e != nil {
		logger.Warn("tun_default_route_scrub tun=%s ifIndex=%d: %v", configName, idx, e)
	}
}

// PreferVPNAfterSoftIPReplace 软换 VPN IP 轻量 PreferVPN（SkipAsSource + iphlp scrub）。
func PreferVPNAfterSoftIPReplace(ctx context.Context, configName, vpnIP string) error {
	idx, err := winnet.InterfaceIndex(configName)
	if err != nil || idx <= 0 {
		return fmt.Errorf("PreferVPNAfterSoftIPReplace: resolve %s: %w", configName, err)
	}
	return winnet.PreferVPNAfterSoftIPReplace(ctx, configName, idx, vpnIP)
}

// ReplaceTUNIPv4KeepICS 在已打开的 TUN 上替换式配 VPN IP，保留 192.168.137.*（软换不拆 ICS）。
//
// prefixLen：无 ICS 时用 32；有 ICS（has_137）时应用 24，与冷启 ics_prefix_keep 一致，禁止软换强制 /32。
func ReplaceTUNIPv4KeepICS(configName, wantIP string, prefixLen int) (removed []string, kept string, err error) {
	idx, err := winnet.InterfaceIndex(configName)
	if err != nil || idx <= 0 {
		return nil, "", fmt.Errorf("ReplaceTUNIPv4KeepICS: resolve %s: %w", configName, err)
	}
	return winnet.ReplaceInterfaceIPv4KeepICS(idx, wantIP, prefixLen)
}

// 门面刻意不导出 DisableAllICS / DisableICSPair：
// 生产路径由 Teardown→disableICSPlatform 承担；再导出易误导旁路编排。
