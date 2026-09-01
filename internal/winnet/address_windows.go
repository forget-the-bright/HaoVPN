//go:build windows

package winnet

import (
	"context"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

// SetInterfaceIPv4 为指定接口配置静态 IPv4 地址与子网掩码。
//
// 参数：ifName — netsh 接口别名；ip、mask — 点分十进制。
// 返回：netsh 失败时 error。
func SetInterfaceIPv4(ifName, ip, mask string) error {
	return RunNetsh("interface", "ipv4", "set", "address",
		"name="+ifName,
		"source=static",
		"addr="+ip,
		"mask="+mask,
	)
}

// DisableInterfaceIPv6 关闭指定接口的 IPv6 管理状态，减少 TUN 侧多余探测流量。
func DisableInterfaceIPv6(ifName string) error {
	return RunNetsh("interface", "ipv6", "set", "interface", "interface="+ifName, "admin=disabled")
}

// AssignIPv4PowerShell 通过 New-NetIPAddress 为 Wintun 配置 IPv4（netsh 失败时的回退路径）。
//
// 参数：
//   configName — TUN 配置名，用于匹配 Get-NetAdapter。
//   ip — 客户端 VPN IPv4。
//   prefix — 前缀长度（如 24）。
// 返回：找不到网卡或 PowerShell 报错时 error。
func AssignIPv4PowerShell(configName, ip string, prefix int) error {
	ps := PSSnippetAssignIPv4(configName, ip, prefix)
	_, err := RunPSOneShot(ps)
	return err
}

// PreferVPNSourceWithICSContext 在保留 ICS 私网地址的前提下，强制本机发包源为 VPN IP；ctx 取消时 Kill PowerShell。
//
// 根因（错源，不是 soft 重连）：ICS 在 TUN 上另挂 192.168.137.1 后，Windows 源地址选择可能不用
// 10.88.x.x，导致本机经隧道访问服务端 AllowedIPs（如 192.168.3.1）超时；对 ICS 地址设 SkipAsSource=$true。
// 热路径：ICS Enable 同脚本内嵌 PSSnippetPreferVPNAfterICS；本函数供回退/单测。
func PreferVPNSourceWithICSContext(ctx context.Context, configName, vpnIP string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("PreferVPNSourceWithICS: configName/vpnIP 为空")
	}
	start := time.Now()
	ps := PSAssignAdapterAndPreferVPN(configName, vpnIP, 0)
	out, err := RunPSOneShotContext(ctx, ps)
	logger.Info("ics_prefer_vpn embedded=false method=standalone elapsed=%s err=%v", time.Since(start), err)
	if err != nil {
		return platform.CommandOutputError("PreferVPNSourceWithICS", out, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ics_src_diag "),
			strings.HasPrefix(line, "ics_prefix_fix "),
			strings.HasPrefix(line, "ics_default_route_scrubbed "),
			strings.HasPrefix(line, "ics_prefer_vpn "):
			logger.Info("windows: %s", line)
		}
	}
	return nil
}

// RemoveICSAddressesKeepVPN 关闭 ICS 后删除所有非 VPN 地址（含 137 与旧 VPN IP），保留 vpnIP。
//
// 参数：configName — TUN 名；vpnIP — 须保留的地址。
// 关联：via Teardown / cleanupTUNAfterViaDisabled(hadVia)；有残留且需关共享时优先 CleanupICSResidue（一次 PS）。
func RemoveICSAddressesKeepVPN(configName, vpnIP string) error {
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("RemoveICSAddressesKeepVPN: configName/vpnIP 为空")
	}
	ps := PSSnippetRemoveNonVPNKeepVPN(vpnIP, PSSnippetAssignAdapterIf(configName))
	_, err := RunPSOneShot(ps)
	return err
}
