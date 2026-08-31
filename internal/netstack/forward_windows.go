//go:build windows

package netstack

import (
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
	"haovpn/internal/winnet"
)

// enableIPForwardPlatform 打开系统与相关网卡的 IPv4 转发。
//
// 步骤：若注册表已 IPEnableRouter=1 则跳过；否则写注册表，再尽力对已连接网卡
// Set-NetIPInterface Forwarding。PowerShell 经 winnet.RunPSBestEffort（Bypass）；
// 失败不阻断（注册表已成功即可）。
func enableIPForwardPlatform() error {
	if ipForwardEnabled() {
		logger.Info("windows: IP 转发已开启，跳过重复配置")
		return nil
	}
	cmd := platform.Command("reg", "add",
		`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`,
		"/v", "IPEnableRouter", "/t", "REG_DWORD", "/d", "1", "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return platform.CommandOutputError("reg IPEnableRouter", out, err)
	}
	ps := `Get-NetIPInterface -AddressFamily IPv4 | Where-Object {$_.ConnectionState -eq 'Connected'} | ForEach-Object { Set-NetIPInterface -InterfaceIndex $_.InterfaceIndex -Forwarding Enabled -ErrorAction SilentlyContinue }`
	winnet.RunPSBestEffort(ps, "Set-NetIPInterface-Forwarding")
	logger.Info("windows: IPEnableRouter=1，已尝试启用网卡 Forwarding")
	return nil
}

// ipForwardEnabled 检查注册表 IPEnableRouter 是否已为 1。
func ipForwardEnabled() bool {
	out, err := platform.Command("reg", "query",
		`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`,
		"/v", "IPEnableRouter").CombinedOutput()
	if err != nil {
		return false
	}
	s := strings.ToLower(string(out))
	return strings.Contains(s, "0x1") ||
		(strings.Contains(s, "ipeablerouter") && strings.Contains(s, " 0x1"))
}
