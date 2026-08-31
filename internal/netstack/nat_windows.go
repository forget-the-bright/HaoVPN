//go:build windows

package netstack

import (
	"fmt"
	"net"
	"strings"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/winnet"
)

// setupNATPlatform 为 VPN 子网访问 LAN 配置 SNAT。
// 优先 WinNAT（New-NetNat，需 Hyper-V）；家庭版等无 WinNAT 时回退 ICS（见 ics_nat_windows.go）。
func setupNATPlatform(vpnSubnet, lanCIDR, tunName string, tunIP net.IP, outboundIf string) error {
	winErr := setupWinNAT(vpnSubnet)
	if winErr == nil {
		return nil
	}
	if isWinNATUnavailable(winErr) {
		logger.Warn("WinNAT 不可用（Windows 家庭版或未启用 Hyper-V）: %v", winErr)
		logger.Info("尝试 ICS 回退（Internet 连接共享）…")
		return setupICSPlatform(tunName, lanCIDR, outboundIf, tunIP)
	}
	return winErr
}

// setupWinNAT 使用 New-NetNat（依赖 Hyper-V/WinNAT 子系统）。
//
// 若已有同名同 prefix 规则则跳过；否则尽力 Remove 旧规则再 New。
// PowerShell 一律经 winnet.RunPS / RunPSBestEffort。
func setupWinNAT(vpnSubnet string) error {
	name := brand.WinNATName
	if winNATMatches(name, vpnSubnet) {
		logger.Info("windows: NetNat %s 已存在 prefix=%s，跳过", name, vpnSubnet)
		return nil
	}
	winnet.RunPSBestEffort(
		fmt.Sprintf("Remove-NetNat -Name %s -Confirm:$false -ErrorAction SilentlyContinue", name),
		"Remove-NetNat-before-New",
	)

	ps := fmt.Sprintf(
		`New-NetNat -Name %s -InternalIPInterfaceAddressPrefix %s -ErrorAction Stop`,
		name, vpnSubnet,
	)
	out, err := winnet.RunPS(ps)
	if err != nil {
		return platform.CommandOutputError("New-NetNat", out, err)
	}
	logger.Info("windows: New-NetNat %s prefix=%s", name, vpnSubnet)
	return nil
}

// isWinNATUnavailable 判断是否为 WinNAT 子系统缺失（Invalid class / 0x80041010）。
func isWinNATUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "0x80041010") ||
		strings.Contains(msg, "Invalid class") ||
		strings.Contains(msg, "无效") ||
		strings.Contains(msg, "Provider load failure") ||
		strings.Contains(msg, "0x80041013")
}

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = vpnSubnet
	_ = lanCIDR
	_ = tunName
	// 尽力删 WinNAT；ICS 由 Teardown 末尾 disableICSPlatform 统一关一次，避免多 LAN 重复 COM。
	winnet.RunPSBestEffort(
		`Remove-NetNat -Name `+brand.WinNATName+` -Confirm:$false -ErrorAction SilentlyContinue`,
		"Remove-NetNat-teardown",
	)
	return nil
}

// winNATMatches 检查是否已有相同 prefix 的 NetNat 规则（避免每次重启 Remove+New）。
func winNATMatches(name, prefix string) bool {
	ps := fmt.Sprintf(`
$n = Get-NetNat -Name '%s' -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $n) { exit 1 }
if ($n.InternalIPInterfaceAddressPrefix -eq '%s') { exit 0 }
exit 2
`, winnet.EscapeSingleQuoted(name), winnet.EscapeSingleQuoted(prefix))
	// exit 0 = 匹配；非零（含 exit 1/2）经 RunPS 变为 error → false。
	_, err := winnet.RunPS(ps)
	return err == nil
}
