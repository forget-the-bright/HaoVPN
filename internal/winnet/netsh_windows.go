//go:build windows

package winnet

import (
	"fmt"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/platform"
)

// RunPS 执行 PowerShell 脚本（NoProfile、Bypass 执行策略、无控制台窗口）。
//
// 参数：script — 完整 -Command 脚本体。
// 返回：CombinedOutput；失败时 error 含 stderr 摘要。
// 副作用：启动 powershell.exe 子进程。
func RunPS(script string) ([]byte, error) {
	out, err := platform.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return out, platform.CommandOutputError("powershell", out, err)
	}
	return out, nil
}

// EscapeSingleQuoted 转义嵌入 PowerShell 单引号字符串字面量的内容。
//
// 参数：s — 待嵌入 '...' 的原始文本。
// 返回：将单引号替换为 '' 后的字符串。
func EscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// RunNetsh 执行 netsh 子命令并在失败时格式化错误信息。
//
// 参数：args — netsh 后续参数（如 interface ipv4 set address ...）。
// 返回：netsh 非零退出或输出含错误时 error。
// 副作用：启动 netsh.exe 子进程，可能修改网络配置（依子命令而定）。
func RunNetsh(args ...string) error {
	out, err := platform.Command("netsh", args...).CombinedOutput()
	if err != nil {
		return platform.CommandOutputError("netsh "+strings.Join(args, " "), out, err)
	}
	return nil
}

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

// SetInterfaceDNSStatic 将接口主 DNS（index=1）设为静态地址。
//
// 参数：ifName — netsh 别名；server — 单个 IPv4 DNS。
func SetInterfaceDNSStatic(ifName, server string) error {
	return RunNetsh("interface", "ipv4", "set", "dnsservers", ifName,
		"source=static", "address="+server, "register=none", "validate=no")
}

// AddInterfaceDNS 向接口追加次级 DNS 服务器。
//
// 参数：index — netsh DNS 优先级（通常从 2 起）；server — IPv4 地址。
func AddInterfaceDNS(ifName, server string, index int) error {
	return RunNetsh("interface", "ipv4", "add", "dnsservers", ifName, server,
		"index="+fmt.Sprintf("%d", index), "validate=no")
}

// RestoreInterfaceDNSDHCP 将接口 DNS 恢复为 DHCP 自动获取。
func RestoreInterfaceDNSDHCP(ifName string) error {
	return RunNetsh("interface", "ipv4", "set", "dnsservers", ifName, "source=dhcp")
}

// ShowInterfaceDNS 读取 netsh interface ipv4 show dnsservers 的原始输出。
//
// 返回：stdout 字节；netsh 失败时 error 与部分输出一并返回。
func ShowInterfaceDNS(ifName string) ([]byte, error) {
	out, err := platform.Command("netsh", "interface", "ipv4", "show", "dnsservers", ifName).CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
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

// DisableAllICS 关闭本机全部 ICS 共享（via 关闭或 Teardown 时清残留）。
//
// 通过 PowerShell COM 枚举连接并 DisableSharing，常见耗时数秒；调用方勿在 UI 线程同步执行。
func DisableAllICS() {
	start := time.Now()
	ps := `
$ErrorActionPreference = 'SilentlyContinue'
regsvr32 /s hnetcfg.dll
$net = New-Object -ComObject HNetCfg.HNetShare
foreach ($c in @($net.EnumEveryConnection())) {
  try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {}
}
`
	_ = platform.Command("powershell", "-NoProfile", "-Command", ps).Run()
	logger.Info("DisableAllICS elapsed=%s", time.Since(start))
}
