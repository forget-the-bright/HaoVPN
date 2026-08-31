//go:build windows

package winnet

import (
	"fmt"
	"strings"

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
	ps := fmt.Sprintf(`
$if = Get-NetAdapter | Where-Object { $_.Name -eq '%s' } | Select-Object -First 1
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } | Select-Object -First 1
}
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress '%s' -PrefixLength %d -ErrorAction Stop | Out-Null
`, EscapeSingleQuoted(configName), ip, prefix)
	_, err := RunPS(ps)
	return err
}

// PreferVPNSourceWithICS 在保留 ICS 私网地址（常 192.168.137.1）的前提下，强制本机发包源为 VPN IP。
//
// 根因：ICS 在 TUN 上另挂 192.168.137.1 后，Windows 源地址选择可能不用 10.88.x.x，
// 导致本机经隧道访问服务端 AllowedIPs（如 192.168.3.1）超时；对 ICS 地址设 SkipAsSource=$true。
//
// 参数：configName — TUN 配置名；vpnIP — 须保留为发包源的 VPN IPv4。
// 关联：netstack setupICSPlatform、clientapp via_exit；与 RemoveICSAddressesKeepVPN 互补（彼删地址、此保地址改 SkipAsSource）。
func PreferVPNSourceWithICS(configName, vpnIP string) error {
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("PreferVPNSourceWithICS: configName/vpnIP 为空")
	}
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$vpn = '%s'
$if = Get-NetAdapter | Where-Object { $_.Name -eq '%s' } | Select-Object -First 1
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } | Select-Object -First 1
}
if (-not $if) { throw '未找到 Wintun 网卡' }
$idx = $if.ifIndex
# 确保 VPN IP 仍在（ICS 有时冲掉 /32）
$hasVpn = Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -eq $vpn }
if (-not $hasVpn) {
  New-NetIPAddress -InterfaceIndex $idx -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
}
# VPN IP 可作为源；其余地址（含 ICS 192.168.137.x）禁止作本机发包源
Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  if ($_.IPAddress -eq $vpn) {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $false -ErrorAction SilentlyContinue
  } else {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $true -ErrorAction SilentlyContinue
  }
}
`, EscapeSingleQuoted(vpnIP), EscapeSingleQuoted(configName))
	out, err := RunPS(ps)
	if err != nil {
		return platform.CommandOutputError("PreferVPNSourceWithICS", out, err)
	}
	return nil
}

// RemoveICSAddressesKeepVPN 关闭 ICS 后删除 192.168.137.x 等 ICS 地址，保留 VPN IP。
//
// 参数：configName — TUN 名；vpnIP — 须保留的地址。
// 关联：via Teardown / cleanupTUNAfterViaDisabled(hadVia)；有残留且需关共享时优先 CleanupICSResidue（一次 PS）。
func RemoveICSAddressesKeepVPN(configName, vpnIP string) error {
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("RemoveICSAddressesKeepVPN: configName/vpnIP 为空")
	}
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$vpn = '%s'
$if = Get-NetAdapter | Where-Object { $_.Name -eq '%s' } | Select-Object -First 1
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } | Select-Object -First 1
}
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 |
  Where-Object { $_.IPAddress -ne $vpn -and $_.IPAddress -like '192.168.137.*' } |
  ForEach-Object { Remove-NetIPAddress -InterfaceIndex $_.InterfaceIndex -IPAddress $_.IPAddress -Confirm:$false -ErrorAction SilentlyContinue }
$has = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 | Where-Object { $_.IPAddress -eq $vpn }
if (-not $has) {
  New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
}
`, EscapeSingleQuoted(vpnIP), EscapeSingleQuoted(configName))
	_, err := RunPS(ps)
	return err
}
