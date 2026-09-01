package winnet

import (
	"fmt"

	"haovpn/internal/brand"
	"haovpn/internal/netutil"
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

// PSSnippetSharedAccessEnsure 确保 SharedAccess 在跑，且已在跑时不 Restart-Force。
//
// 为何：旧脚本无条件 Restart-Service -Force，会抖 public 网卡上的 TCP（家用 WLAN+隧道同卡
// → ICS 窗口 soft 重连 / 服务端 replay）。服务已 Running 时只需保证 Manual，勿重启。
// EnableSharing 仍失败时由调用方再 Restart 一次（见 setupICSWithPublicIf 重试环）。
// 输出：ics_sharedaccess action=start|already_running（供 Go 侧 Info 日志）。
func PSSnippetSharedAccessEnsure() string {
	return `Set-Service SharedAccess -StartupType Manual -ErrorAction SilentlyContinue
$sa = Get-Service SharedAccess -ErrorAction SilentlyContinue
if ($sa -and $sa.Status -eq 'Running') {
  Write-Output 'ics_sharedaccess action=already_running'
} else {
  Start-Service SharedAccess -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 1
  Write-Output 'ics_sharedaccess action=start'
}`
}

// PSSnippetSharedAccessRestart 强制重启 SharedAccess（仅 EnableSharing 失败后的补救路径）。
//
// 会抖网卡；禁止作默认路径。输出 ics_sharedaccess action=restart。
func PSSnippetSharedAccessRestart() string {
	return `Set-Service SharedAccess -StartupType Manual -ErrorAction SilentlyContinue
Restart-Service SharedAccess -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Write-Output 'ics_sharedaccess action=restart'`
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

// PSSnippetPreferVPNAfterICS 嵌入 ICS Enable 成功后的 PreferVPN：短轮询 137 → 恢复主机 /32 → SkipAsSource → 清 TUN 默认路由。
//
// 参数：vpnIP — TUN VPN 地址；tunIfIndex — TUN ifIndex（>0 优先，否则用调用方已设的 $prvIdx）。
// 假定脚本中 $prvIdx 已设（与 setupICSWithPublicIf 一致）；本片段用 $vpn/$idx。
//
// 为何恢复 /32：ICS EnableSharing 可能把网卡主机地址从 Open/assign 的 vpn_ip/32 扩成 /24；
// 这不是握手 AllowedIPs（分流路由仍按握手装）。主机 /24 会触发 Windows 连接路由副作用，并常伴随
// TUN 上 0.0.0.0/0 跃点 5。本片段只纠正主机前缀并清该 ifIndex 的默认路由，不动物理网关。
//
// 输出：ics_prefer_vpn wait_ms=…；ics_prefix_fix（若曾扩前缀）；ics_default_route_scrubbed count=…；ics_src_diag…
func PSSnippetPreferVPNAfterICS(vpnIP string, tunIfIndex int) string {
	icsWild := EscapeSingleQuoted(netutil.ICSPrivateIPv4Wildcard())
	return fmt.Sprintf(`
$vpn = '%s'
$idx = %d
if ($idx -le 0) { $idx = $prvIdx }
$waitMs = 0
$deadline = [Environment]::TickCount64 + 1500
while ([Environment]::TickCount64 -lt $deadline) {
  $has137 = Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -like '%s' }
  if ($has137) { break }
  Start-Sleep -Milliseconds 100
  $waitMs += 100
}
$hasVpn = Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -eq $vpn } | Select-Object -First 1
if (-not $hasVpn) {
  New-NetIPAddress -InterfaceIndex $idx -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
} elseif ([int]$hasVpn.PrefixLength -ne 32) {
  $oldPrefix = [int]$hasVpn.PrefixLength
  Remove-NetIPAddress -InterfaceIndex $idx -IPAddress $vpn -Confirm:$false -ErrorAction SilentlyContinue
  New-NetIPAddress -InterfaceIndex $idx -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
  Write-Output ('ics_prefix_fix old=' + $oldPrefix + ' new=32 ip=' + $vpn)
}
# 纵深：非 vpn 且非 ICS 137 的残留（如在线改 VPN IP 留下的旧地址）直接删除，禁止仅 SkipAsSource 掩盖
Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  if ($_.IPAddress -eq $vpn) {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $false -ErrorAction SilentlyContinue
  } elseif ($_.IPAddress -like '%s') {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $true -ErrorAction SilentlyContinue
  } else {
    Remove-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -Confirm:$false -ErrorAction SilentlyContinue
  }
}
# 仅清本 TUN ifIndex 上的默认路由（ICS 注入跃点 5）；禁止动 WLAN/以太网 0.0.0.0/0
$scrubbed = 0
Get-NetRoute -InterfaceIndex $idx -DestinationPrefix '0.0.0.0/0' -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  $_ | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue
  $scrubbed++
}
Write-Output ('ics_prefer_vpn wait_ms=' + $waitMs)
Write-Output ('ics_default_route_scrubbed count=' + $scrubbed)
Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  Write-Output ('ics_src_diag ip=' + $_.IPAddress + ' prefix=' + $_.PrefixLength + ' skip=' + $_.SkipAsSource)
}
`, EscapeSingleQuoted(vpnIP), tunIfIndex, icsWild, icsWild)
}

// PSSnippetSkipAsSourceOnly 软换 VPN IP 后轻量 PreferVPN：仅刷新 SkipAsSource（Replace 已做 /32 与删旧 IP）。
//
// 假定 $idx 已设；无 137 轮询、无 prefix fix、无 Remove 杂地址、无清默认路由（Go iphlp 已做）。
func PSSnippetSkipAsSourceOnly(vpnIP string, tunIfIndex int) string {
	icsWild := EscapeSingleQuoted(netutil.ICSPrivateIPv4Wildcard())
	return fmt.Sprintf(`
$vpn = '%s'
$idx = %d
Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  if ($_.IPAddress -eq $vpn) {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $false -ErrorAction SilentlyContinue
  } elseif ($_.IPAddress -like '%s') {
    Set-NetIPAddress -InterfaceIndex $idx -IPAddress $_.IPAddress -SkipAsSource $true -ErrorAction SilentlyContinue
  }
}
Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object {
  Write-Output ('ics_src_diag ip=' + $_.IPAddress + ' prefix=' + $_.PrefixLength + ' skip=' + $_.SkipAsSource)
}
`, EscapeSingleQuoted(vpnIP), tunIfIndex, icsWild)
}

// PSSnippetICSAlreadyPairedCheck 在已解析 $pub/$prv 且拿到 $pubCfg/$prvCfg 后，
// 若 public=共享因特网(0)、private=专用(1) 均已启用，则置 $ok=true 并输出 already_paired。
//
// 嵌入位置：取得 cfg 之后、Try-EnableICS 之前。不匹配时 $ok 保持 false，走完整 Enable。
// 属性名：部分系统为 SharingType，部分为 SharingConnectionType——两者都试。
func PSSnippetICSAlreadyPairedCheck() string {
	return `
$script:ok = $false
function Get-IcsShareType($cfg) {
  try { return [int]$cfg.SharingType } catch {}
  try { return [int]$cfg.SharingConnectionType } catch {}
  return -1
}
try {
  if ($pubCfg.SharingEnabled -and $prvCfg.SharingEnabled) {
    $pt = Get-IcsShareType $pubCfg
    $vt = Get-IcsShareType $prvCfg
    if ($pt -eq 0 -and $vt -eq 1) {
      $script:ok = $true
      Write-Output 'ics_enable action=already_paired'
    }
  }
} catch {}
`
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

// PSSnippetRemoveNetNat 生成 Remove-NetNat 脚本（Name 单引号 + EscapeSingleQuoted）。
//
// 参数：name — WinNAT 规则名（通常 brand.WinNATName）。
// 为何在 winnet：与 ICS PreferVPN 等模板同属 PS 单一真相源；netstack 只编排何时 Remove。
// 关联：setupWinNAT 清旧、teardownNATPlatform。
func PSSnippetRemoveNetNat(name string) string {
	return fmt.Sprintf(
		"Remove-NetNat -Name '%s' -Confirm:$false -ErrorAction SilentlyContinue",
		EscapeSingleQuoted(name),
	)
}

// PSSnippetNewNetNat 生成 New-NetNat 脚本（Name 与 InternalIP 前缀均 EscapeSingleQuoted）。
//
// 参数：name — 规则名；vpnSubnet — 如 10.88.0.0/24（来自配置/握手，不可裸拼）。
// 安全：握手下发子网若含 ' 可破坏脚本；禁止业务包 fmt 裸 %s。
func PSSnippetNewNetNat(name, vpnSubnet string) string {
	return fmt.Sprintf(
		`New-NetNat -Name '%s' -InternalIPInterfaceAddressPrefix '%s' -ErrorAction Stop`,
		EscapeSingleQuoted(name),
		EscapeSingleQuoted(vpnSubnet),
	)
}

// PSSnippetEnableIPv4Forwarding 对已连接 IPv4 网卡启用 Forwarding（IPEnableRouter 已写注册表后的补充）。
func PSSnippetEnableIPv4Forwarding() string {
	return `Get-NetIPInterface -AddressFamily IPv4 | Where-Object {$_.ConnectionState -eq 'Connected'} | ForEach-Object { Set-NetIPInterface -InterfaceIndex $_.InterfaceIndex -Forwarding Enabled -ErrorAction SilentlyContinue }`
}

// PSSnippetGetNetNatMatch 检查 NetNat 规则是否存在且 prefix 一致；stdout 为 MISS/MATCH/DIFF。
func PSSnippetGetNetNatMatch(name, prefix string) string {
	return fmt.Sprintf(`
$n = Get-NetNat -Name '%s' -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $n) { Write-Output 'MISS' }
elseif ($n.InternalIPInterfaceAddressPrefix -eq '%s') { Write-Output 'MATCH' }
else { Write-Output 'DIFF' }
`, EscapeSingleQuoted(name), EscapeSingleQuoted(prefix))
}

// PSSnippetAssignIPv4 为 Wintun 配置 IPv4（先清旧地址再 New-NetIPAddress）。
func PSSnippetAssignIPv4(configName, ip string, prefix int) string {
	return fmt.Sprintf(`
%s
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress '%s' -PrefixLength %d -ErrorAction Stop | Out-Null
`, PSSnippetAssignAdapterIf(configName), EscapeSingleQuoted(ip), prefix)
}

// PSSnippetRemoveNonVPNKeepVPN 删除 TUN 上除 vpnIP 外全部 IPv4，并确保 vpn /32 存在。
func PSSnippetRemoveNonVPNKeepVPN(vpnIP string, assignAdapterSnippet string) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$vpn = '%s'
%s
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 |
  Where-Object { $_.IPAddress -ne $vpn } |
  ForEach-Object { Remove-NetIPAddress -InterfaceIndex $_.InterfaceIndex -IPAddress $_.IPAddress -Confirm:$false -ErrorAction SilentlyContinue }
$has = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 | Where-Object { $_.IPAddress -eq $vpn }
if (-not $has) {
  New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
}
`, EscapeSingleQuoted(vpnIP), assignAdapterSnippet)
}

// PSSnippetProbeICSResidue 探测 TUN 是否仍有 ICS 私网地址（stdout 1/0）。
func PSSnippetProbeICSResidue(configName string) string {
	wild := netutil.ICSPrivateIPv4Wildcard()
	return "$ErrorActionPreference = 'SilentlyContinue'\n" +
		PSSnippetAssignAdapterIf(configName) + fmt.Sprintf(`
if (-not $if) { Write-Output '0' }
else {
$hit = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -like '%s' } | Select-Object -First 1
if ($hit) { Write-Output '1' } else { Write-Output '0' }
}
`, EscapeSingleQuoted(wild))
}

// PSSnippetVerifyInterfaceExists 校验网卡别名存在且 Up（失败 throw）。
func PSSnippetVerifyInterfaceExists(name string) string {
	return fmt.Sprintf(`
$n = '%s'
$ok = $false
Get-NetIPAddress -AddressFamily IPv4 | ForEach-Object {
  if ($_.InterfaceAlias -eq $n) { $ok = $true }
}
if (-not $ok) {
  Get-NetAdapter | ForEach-Object { if ($_.Name -eq $n -and $_.Status -eq 'Up') { $ok = $true } }
}
if (-not $ok) { throw "网卡不存在或未 Up: $n" }
`, EscapeSingleQuoted(name))
}

// PSSnippetScrubDefaultRoute 删除指定 ifIndex 上 IPv4 默认路由；stdout count=N。
func PSSnippetScrubDefaultRoute(ifIndex int) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
$idx = %d
$n = 0
Get-NetRoute -InterfaceIndex $idx -DestinationPrefix '0.0.0.0/0' -AddressFamily IPv4 |
  ForEach-Object { $_ | Remove-NetRoute -Confirm:$false; $n++ }
Write-Output ('count=' + $n)
`, ifIndex)
}

// PSSnippetICSEnableSharing COM EnableSharing 主脚本（TUN 私网 + 公网侧 ICS）。
//
// 参数 preClear / alreadyPairedCheck / preferSnippet 为调用方拼好的片段（可为空）。
// sharedAccessEnsure / disableLoop / sharedAccessRestart 由本函数内嵌固定 snippet。
func PSSnippetICSEnableSharing(pubName, prvName string, tunIfIndex int, tunAlias, preClear, alreadyPairedCheck, preferSnippet string) string {
	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
regsvr32 /s hnetcfg.dll
Get-CimInstance -Namespace ROOT/Microsoft/HomeNet -ClassName HNet_ConnectionProperties -ErrorAction SilentlyContinue |
  ForEach-Object { if ($_.IsIcsPrivate) { Set-CimInstance -InputObject $_ -Property @{ IsIcsPrivate = $false } -ErrorAction SilentlyContinue } }
$net = New-Object -ComObject HNetCfg.HNetShare
%s
netsh wlan stop hostednetwork 2>$null | Out-Null
%s
$pubName = '%s'
$prvName = '%s'
$prvIdx = %d
$prvAlias = '%s'
$pub = $null
$prv = $null
foreach ($c in @($net.EnumEveryConnection())) {
  $n = $net.NetConnectionProps($c).Name
  if ($n -eq $pubName -or ($n -like "*$pubName*")) { $pub = $c }
  if ($n -eq $prvName -or ($n -like "*$prvName*") -or ($prvAlias -ne '' -and $n -eq $prvAlias)) { $prv = $c }
}
if (-not $prv -and $prvIdx -gt 0) {
  $na = Get-NetAdapter -InterfaceIndex $prvIdx -ErrorAction SilentlyContinue
  if ($na) {
    foreach ($c in @($net.EnumEveryConnection())) {
      if ($net.NetConnectionProps($c).Name -eq $na.Name) { $prv = $c; break }
    }
  }
}
if (-not $pub) { throw "ICS: 未找到出站网卡 $pubName" }
if (-not $prv) { throw "ICS: 未找到 TUN 网卡 $prvName（Wintun 须已创建）" }
$pubCfg = $net.INetSharingConfigurationForINetConnection($pub)
$prvCfg = $net.INetSharingConfigurationForINetConnection($prv)
%s
if (-not $ok) {
  function Try-EnableICS {
    $script:ok = $false
    foreach ($order in @('privateFirst','publicFirst')) {
      %s
      try {
        if ($order -eq 'privateFirst') { $prvCfg.EnableSharing(1); $pubCfg.EnableSharing(0) }
        else { $pubCfg.EnableSharing(0); $prvCfg.EnableSharing(1) }
        $script:ok = $true
        break
      } catch { }
    }
  }
  Try-EnableICS
  if (-not $ok) {
    %s
    Try-EnableICS
  }
}
if (-not $ok) { throw "ICS EnableSharing 失败（0x80040201 常见于 Win11 家庭版，可设 nat.forward_only: true 或手工在「网络连接→共享」启用一次）" }
Write-Output 'ics_enable_ok'
%s
`, preClear, PSSnippetSharedAccessEnsure(),
		EscapeSingleQuoted(pubName), EscapeSingleQuoted(prvName), tunIfIndex, EscapeSingleQuoted(tunAlias),
		alreadyPairedCheck,
		PSSnippetICSDisableSharingLoop(), PSSnippetSharedAccessRestart(),
		preferSnippet)
}

// PSSnippetFindInterfaceInCIDR 在 PS 回退路径中查找本机有指定网段 IP 的网卡别名。
func PSSnippetFindInterfaceInCIDR(network, mask, skipPattern string) string {
	return fmt.Sprintf(`
$network = '%s'
$mask = '%s'
$skip = '%s'
$found = $null
Get-NetIPAddress -AddressFamily IPv4 | Where-Object {
  $_.InterfaceAlias -notmatch $skip -and
  $_.IPAddress -notmatch '^169\.254\.' -and $_.IPAddress -ne '127.0.0.1'
} | ForEach-Object {
  $ip = [System.Net.IPAddress]::Parse($_.IPAddress)
  $net = [System.Net.IPAddress]::Parse($network)
  $m = [System.Net.IPAddress]::Parse($mask)
  $ib = $ip.GetAddressBytes(); $nb = $net.GetAddressBytes(); $mb = $m.GetAddressBytes()
  $ok = $true
  for ($i = 0; $i -lt 4; $i++) {
    if (($ib[$i] -band $mb[$i]) -ne ($nb[$i] -band $mb[$i])) { $ok = $false; break }
  }
  if ($ok) { $found = $_.InterfaceAlias }
}
if ($found) { $found }
`, EscapeSingleQuoted(network), EscapeSingleQuoted(mask), EscapeSingleQuoted(skipPattern))
}

// PSSnippetFindInterfaceByRoute 查路由表找出站网卡；stdout viaDefault|alias。
func PSSnippetFindInterfaceByRoute(probe, skipPattern string) string {
	return fmt.Sprintf(`
$probe = '%s'
$skip = '%s'
$found = $null
$viaDefault = '0'
try {
  $r = Find-NetRoute -RemoteIPAddress $probe -ErrorAction Stop |
    Where-Object { $_.InterfaceAlias -notmatch $skip } |
    Select-Object -First 1
  if ($r) {
    $found = $r.InterfaceAlias
    $pref = [string]$r.DestinationPrefix
    if ($pref -eq '0.0.0.0/0') { $viaDefault = '1' }
  }
} catch {}
if (-not $found) {
  $r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -AddressFamily IPv4 |
    Where-Object { $_.InterfaceAlias -notmatch $skip } |
    Sort-Object RouteMetric |
    Select-Object -First 1
  if ($r) { $found = $r.InterfaceAlias; $viaDefault = '1' }
}
if (-not $found) { throw "路由表无可用出站网卡" }
Write-Output ($viaDefault + '|' + $found)
`, EscapeSingleQuoted(probe), EscapeSingleQuoted(skipPattern))
}
