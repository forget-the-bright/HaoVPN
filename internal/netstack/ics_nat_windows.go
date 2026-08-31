//go:build windows

package netstack

import (
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/winnet"
)

// findOutboundInterface 确定 ICS 公网侧网卡：配置 > 本机同网段 IP > 路由表 > 默认路由。
func findOutboundInterface(lanCIDR, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if err := verifyInterfaceExists(configured); err != nil {
			return "", fmt.Errorf("outbound_interface=%q: %w", configured, err)
		}
		logger.Info("windows: 使用配置的 outbound_interface=%s", configured)
		return configured, nil
	}
	if name, err := findInterfaceWithIPInCIDR(lanCIDR); err == nil && name != "" {
		logger.Info("windows: 出站网卡（本机 LAN IP 同网段）=%s", name)
		return name, nil
	}
	if name, err := findInterfaceByRoute(lanCIDR); err == nil && name != "" {
		logger.Info("windows: 出站网卡（路由表至 %s）=%s", lanCIDR, name)
		return name, nil
	}
	return "", fmt.Errorf("未找到至 %s 的出站网卡（可配置 nat.outbound_interface，如 ZeroTier 网卡名）", lanCIDR)
}

func verifyInterfaceExists(name string) error {
	ps := fmt.Sprintf(`
$n = '%s'
$ok = $false
Get-NetIPAddress -AddressFamily IPv4 | ForEach-Object {
  if ($_.InterfaceAlias -eq $n) { $ok = $true }
}
if (-not $ok) {
  Get-NetAdapter | ForEach-Object { if ($_.Name -eq $n -and $_.Status -eq 'Up') { $ok = $true } }
}
if (-not $ok) { throw "网卡不存在或未 Up: $n" }
`, winnet.EscapeSingleQuoted(name))
	out, err := winnet.RunPSOneShot(ps)
	if err != nil {
		return err
	}
	_ = out
	return nil
}

// findInterfaceWithIPInCIDR 本机有该网段 IP 的网卡（服务端与 PLC 同二层/同网段）。
func findInterfaceWithIPInCIDR(lanCIDR string) (string, error) {
	_, ipnet, err := netutil.ParseCIDR(lanCIDR)
	if err != nil {
		return "", err
	}
	network := ipnet.IP.Mask(ipnet.Mask).String()
	mask := net.IP(ipnet.Mask).String()

	ps := fmt.Sprintf(`
$network = '%s'
$mask = '%s'
$found = $null
Get-NetIPAddress -AddressFamily IPv4 | Where-Object {
  $_.InterfaceAlias -notmatch 'HaoVPN|Loopback|TAP-WIN|TUN' -and
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
`, network, mask)

	out, err := winnet.RunPSOneShot(ps)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// findInterfaceByRoute 查路由表：至 lanCIDR 的下一跳网卡，否则用最低 metric 默认路由。
func findInterfaceByRoute(lanCIDR string) (string, error) {
	probe := ProbeIPForCIDR(lanCIDR)
	ps := fmt.Sprintf(`
$probe = '%s'
$skip = 'HaoVPN|Loopback|TAP-WIN|TUN|OpenVPN|Tailscale'
$found = $null
try {
  $r = Find-NetRoute -RemoteIPAddress $probe -ErrorAction Stop |
    Where-Object { $_.InterfaceAlias -notmatch $skip } |
    Select-Object -First 1
  if ($r) { $found = $r.InterfaceAlias }
} catch {}
if (-not $found) {
  $r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -AddressFamily IPv4 |
    Where-Object { $_.InterfaceAlias -notmatch $skip } |
    Sort-Object RouteMetric |
    Select-Object -First 1
  if ($r) { $found = $r.InterfaceAlias }
}
if (-not $found) { throw "路由表无可用出站网卡" }
$found
`, probe)

	out, err := winnet.RunPSOneShot(ps)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("路由表无可用出站网卡")
	}
	return name, nil
}

// setupICSPlatform 用 ICS 做 VPN→LAN SNAT（WinNAT 不可用时的回退，如 Windows 家庭版）。
// tunIP 为 TUN 上的 VPN/网关地址；启用 ICS 后对其它地址设 SkipAsSource，避免本机错源。
func setupICSPlatform(tunName, lanCIDR, outboundIf string, tunIP net.IP) error {
	lanIf, err := findOutboundInterface(lanCIDR, outboundIf)
	if err != nil {
		return fmt.Errorf("ICS 回退: %w", err)
	}
	logger.Info("windows: ICS 回退 lan_if=%s tun=%s lan=%s", lanIf, tunName, lanCIDR)

	tunIfIndex := 0
	if idx, err := winnet.InterfaceIndex(tunName); err == nil {
		tunIfIndex = idx
	}
	tunAlias := winnet.ResolveInterfaceAlias(tunName)

	ps := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
regsvr32 /s hnetcfg.dll
Get-CimInstance -Namespace ROOT/Microsoft/HomeNet -ClassName HNet_ConnectionProperties -ErrorAction SilentlyContinue |
  ForEach-Object { if ($_.IsIcsPrivate) { Set-CimInstance -InputObject $_ -Property @{ IsIcsPrivate = $false } -ErrorAction SilentlyContinue } }
$net = New-Object -ComObject HNetCfg.HNetShare
%s
netsh wlan stop hostednetwork 2>$null | Out-Null
Set-Service SharedAccess -StartupType Manual -ErrorAction SilentlyContinue
Restart-Service SharedAccess -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
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
$ok = $false
foreach ($order in @('privateFirst','publicFirst')) {
  %s
  try {
    if ($order -eq 'privateFirst') { $prvCfg.EnableSharing(1); $pubCfg.EnableSharing(0) }
    else { $pubCfg.EnableSharing(0); $prvCfg.EnableSharing(1) }
    $ok = $true
    break
  } catch { }
}
if (-not $ok) { throw "ICS EnableSharing 失败（0x80040201 常见于 Win11 家庭版，可设 nat.forward_only: true 或手工在「网络连接→共享」启用一次）" }
`, winnet.PSSnippetICSDisableSharingLoop(), winnet.EscapeSingleQuoted(lanIf), winnet.EscapeSingleQuoted(tunName), tunIfIndex, winnet.EscapeSingleQuoted(tunAlias), winnet.PSSnippetICSDisableSharingLoop())

	if _, err := winnet.RunPSOneShot(ps); err != nil {
		return fmt.Errorf("ICS 启用失败: %w（家庭版请确认 LAN 网卡名正确且 SharedAccess 服务可启动）", err)
	}
	logger.Info("windows: ICS 已启用 public=%s private=%s（VPN→LAN NAT 回退）", lanIf, tunName)
	winnet.RememberICSPair(lanIf, tunName)

	// ICS 异步挂 192.168.137.1，稍等再设 SkipAsSource
	time.Sleep(1500 * time.Millisecond)

	// 根因：ICS 在 TUN 挂 192.168.137.1 后 Windows 可能用它作源；保留该地址供 ICS SNAT，但 SkipAsSource
	if tunIP != nil {
		if v4 := tunIP.To4(); v4 != nil {
			vpn := v4.String()
			if err := winnet.PreferVPNSourceWithICS(tunName, vpn); err != nil {
				logger.Warn("windows: ICS 后 PreferVPNSource 失败（本机 AllowedIPs 可能仍异常）: %v", err)
			} else {
				logger.Info("windows: ICS 后已 SkipAsSource 非 VPN 地址，本机发包源优先 %s", vpn)
			}
		}
	}
	return nil
}

// disableICSPlatform 关闭本会话 ICS（优先靶向网卡对，残留再全机 DisableAllICS）。
func disableICSPlatform() {
	start := time.Now()
	pub, prv, ok := winnet.TakeICSPair()
	if ok {
		winnet.DisableICSPair(pub, prv)
		// 仍有 192.168.137.* 则兜底全关
		tun := prv
		if tun == "" {
			tun = pub
		}
		if winnet.HasICSResidue(tun) {
			logger.Info("windows: DisableICSPair 后仍有 ICS 残留，DisableAllICS 兜底 tun=%s", tun)
			winnet.DisableAllICS()
		}
	} else {
		winnet.DisableAllICS()
	}
	logger.Info("windows: disableICSPlatform elapsed=%s pair=%v", time.Since(start), ok)
}
