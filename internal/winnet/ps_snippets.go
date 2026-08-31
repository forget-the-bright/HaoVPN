package winnet

import (
	"fmt"

	"haovpn/internal/brand"
)

// PSSnippetAssignAdapterIf 生成「按 Name 优先、否则按 Wintun/品牌池描述」查找网卡并赋给 $if 的 PS 片段。
//
// 参数：configName — TUN/yaml 配置名（如 haovpn0）。
// 嵌入安全：Name -eq 仅 EscapeSingleQuoted；-match 操作数先 EscapeRegex 再 EscapeSingleQuoted。
// 返回：可直接拼进更大脚本的片段；调用方之后使用 $if（可能仍为 $null）。
// 为何集中：address/resolver/ics/tun 曾各自复制 Get-NetAdapter 模板，池名或描述一改就会漂移。
// 品牌描述第二段一律 brand.WintunPool（当前为 HaoVPN），禁止业务包再写死 'HaoVPN'。
func PSSnippetAssignAdapterIf(configName string) string {
	return fmt.Sprintf(`$if = Get-NetAdapter | Where-Object { $_.Name -eq '%s' } | Select-Object -First 1
if (-not $if) {
  $if = Get-NetAdapter | Where-Object { $_.InterfaceDescription -match '%s|%s' } | Select-Object -First 1
}`, EscapeSingleQuoted(configName), EscapeSingleQuoted(EscapeRegex("Wintun")), EscapeSingleQuoted(EscapeRegex(brand.WintunPool)))
}

// PSSnippetICSDisableSharingLoop 假定脚本中已有 $net = HNetCfg.HNetShare，枚举并 DisableSharing。
//
// 用于已创建 $net 后的二次清共享（如 ICS Enable 重试前），避免重复 New-Object。
func PSSnippetICSDisableSharingLoop() string {
	return `foreach ($c in @($net.EnumEveryConnection())) {
  try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {}
}`
}

// PSSnippetICSDisableAll 完整「注册 hnetcfg + 新建 HNetShare + 关闭全部 ICS 共享」片段。
//
// 调用方：DisableAllICS、CleanupICSResidue、netstack setupICSPlatform 启用以前的清场。
// 注意：COM 在部分环境耗时数秒；BestEffort 路径勿当硬错误。
func PSSnippetICSDisableAll() string {
	return `regsvr32 /s hnetcfg.dll
$net = New-Object -ComObject HNetCfg.HNetShare
` + PSSnippetICSDisableSharingLoop()
}

// PSSnippetICSDisablePair 仅关闭名称匹配 public/private 两块网卡上的 ICS 共享（快于全机枚举）。
//
// 参数：public — 出站侧友好名；private — TUN 侧配置名；均可空（空则该侧不匹配）。
// 返回：完整可执行脚本体（含 ErrorAction / Escape）。
// 关联：RememberICSPair → DisableICSPair；失败由 RunPSBestEffort 打 Warn。
func PSSnippetICSDisablePair(public, private string) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
regsvr32 /s hnetcfg.dll
$pubName = '%s'
$prvName = '%s'
$net = New-Object -ComObject HNetCfg.HNetShare
foreach ($c in @($net.EnumEveryConnection())) {
  try {
    $n = $net.NetConnectionProps($c).Name
    $hit = $false
    if ($pubName -ne '' -and ($n -eq $pubName -or $n -like ("*" + $pubName + "*"))) { $hit = $true }
    if ($prvName -ne '' -and ($n -eq $prvName -or $n -like ("*" + $prvName + "*"))) { $hit = $true }
    if ($hit) { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() }
  } catch {}
}
`, EscapeSingleQuoted(public), EscapeSingleQuoted(private))
}

// BuildPrepareWintunOrphanScript 生成清理「同名前缀孤儿 Wintun 网卡」的 PowerShell（如 haovpn0 1）。
//
// 参数：configName — 合法 OpenAdapter 名；空字符串返回空脚本（调用方应跳过执行）。
// 返回：完整 -Command 脚本体；含 Remove-NetAdapter；描述匹配 Wintun|WintunPool。
// 嵌入：$want/-eq 仅 EscapeSingleQuoted；-match 池名先 EscapeRegex 再 EscapeSingleQuoted。
// 上游：tun.prepareWintunAdapter；单测钉死含 brand.WintunPool 与 Escape。
func BuildPrepareWintunOrphanScript(configName string) string {
	if configName == "" {
		return ""
	}
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$want = '%s'
$removed = @()
Get-NetAdapter -ErrorAction SilentlyContinue | Where-Object {
  $_.InterfaceDescription -match 'Wintun|%s' -and
  $_.Name -ne $want -and
  ($_.Name -like ($want + '*'))
} | ForEach-Object {
  Remove-NetAdapter -Name $_.Name -Confirm:$false -ErrorAction Stop
  $removed += $_.Name
}
if ($removed.Count -gt 0) {
  Write-Output ('已移除孤儿网卡: ' + ($removed -join ', '))
}
`, EscapeSingleQuoted(configName), EscapeSingleQuoted(EscapeRegex(brand.WintunPool)))
}
