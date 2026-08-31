//go:build windows

package winnet

import (
	"fmt"
	"strings"
	"time"

	"haovpn/internal/logger"
)

// DisableAllICS 关闭本机全部 ICS 共享（via Teardown 或有残留清理时）。
//
// 通过 PowerShell COM 枚举连接并 DisableSharing，常见耗时数秒；调用方勿在 UI 线程同步执行。
// 空 local_lans 登录路径应先 HasICSResidue，无残留勿调用（见 CleanupICSResidue）。
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
	// 尽力关闭：COM 在部分环境会失败，不能阻断 Teardown；失败见 RunPSBestEffort 的 Warn。
	RunPSBestEffort(ps, "DisableAllICS")
	logger.Info("DisableAllICS elapsed=%s", time.Since(start))
}

// HasICSResidue 便宜探测 TUN/Wintun 上是否仍有 ICS 私网地址（192.168.137.*）。
//
// 参数：configName — TUN 配置名（如 haovpn0）；空则仅按 Wintun|HaoVPN 描述匹配。
// 返回：true 表示存在 ICS 地址残留，值得跑慢速 CleanupICSResidue；探测失败当作无残留（false），
// 避免公司机无 via 时因探测异常反复触发十几秒 DisableAllICS。
// 副作用：一次短 PowerShell（Get-NetAdapter/Get-NetIPAddress），通常亚秒级。
// 关联：clientapp/via_exit.go cleanupTUNAfterViaDisabled。
func HasICSResidue(configName string) bool {
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$name = '%s'
$if = $null
if ($name -ne '') {
  $if = Get-NetAdapter | Where-Object { $_.Name -eq $name } | Select-Object -First 1
}
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match 'Wintun|HaoVPN' } | Select-Object -First 1
}
if (-not $if) { Write-Output '0'; exit 0 }
$hit = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -like '192.168.137.*' } | Select-Object -First 1
if ($hit) { Write-Output '1' } else { Write-Output '0' }
`, EscapeSingleQuoted(configName))
	out, err := RunPS(ps)
	if err != nil {
		logger.Debug("HasICSResidue probe fail tun=%s: %v", configName, err)
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// CleanupICSResidue 一次 PowerShell：全机关闭 ICS 共享并删除 TUN 上 192.168.137.*，保留 vpnIP。
//
// 参数：configName — TUN 名；vpnIP — 须保留的 VPN 地址。
// 返回：PS 失败时 error（尽力清理，调用方打 Warn 即可）。
// 为何合并：空 local_lans 有残留时原先 DisableAllICS + RemoveICSAddresses 各起一次进程，白白加倍开销。
// 关联：仅在 HasICSResidue 为 true 或调用方确认有残留时调用；Teardown 仍可单独 DisableAllICS。
func CleanupICSResidue(configName, vpnIP string) error {
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("CleanupICSResidue: configName/vpnIP 为空")
	}
	start := time.Now()
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
regsvr32 /s hnetcfg.dll
$net = New-Object -ComObject HNetCfg.HNetShare
foreach ($c in @($net.EnumEveryConnection())) {
  try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {}
}
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
	logger.Info("CleanupICSResidue elapsed=%s tun=%s keep=%s err=%v", time.Since(start), configName, vpnIP, err)
	return err
}
