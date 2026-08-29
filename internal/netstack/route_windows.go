//go:build windows

package netstack

import (
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/brand"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/platform"
	"haovpn/internal/winnet"
)

// ifIndex 解析已统一到 internal/winnet（避免 netstack→tun 反向依赖）。

// enableIPForwardPlatform 打开系统与相关网卡的 IPv4 转发。
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
	_ = platform.Command("powershell", "-NoProfile", "-Command", ps).Run()
	logger.Info("windows: IPEnableRouter=1，已尝试启用网卡 Forwarding")
	return nil
}

// setupNATPlatform 为 VPN 子网访问 LAN 配置 SNAT。
// 优先 WinNAT（New-NetNat，需 Hyper-V）；家庭版等无 WinNAT 时回退 ICS。
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
func setupWinNAT(vpnSubnet string) error {
	name := brand.WinNATName
	if winNATMatches(name, vpnSubnet) {
		logger.Info("windows: NetNat %s 已存在 prefix=%s，跳过", name, vpnSubnet)
		return nil
	}
	_ = platform.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("Remove-NetNat -Name %s -Confirm:$false -ErrorAction SilentlyContinue", name)).Run()

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
	out, err := winnet.RunPS(ps)
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

	out, err := winnet.RunPS(ps)
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

	out, err := winnet.RunPS(ps)
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
foreach ($c in @($net.EnumEveryConnection())) {
  try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {}
}
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
  foreach ($c in @($net.EnumEveryConnection())) { try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {} }
  try {
    if ($order -eq 'privateFirst') { $prvCfg.EnableSharing(1); $pubCfg.EnableSharing(0) }
    else { $pubCfg.EnableSharing(0); $prvCfg.EnableSharing(1) }
    $ok = $true
    break
  } catch { }
}
if (-not $ok) { throw "ICS EnableSharing 失败（0x80040201 常见于 Win11 家庭版，可设 nat.forward_only: true 或手工在「网络连接→共享」启用一次）" }
`, winnet.EscapeSingleQuoted(lanIf), winnet.EscapeSingleQuoted(tunName), tunIfIndex, winnet.EscapeSingleQuoted(tunAlias))

	if _, err := winnet.RunPS(ps); err != nil {
		return fmt.Errorf("ICS 启用失败: %w（家庭版请确认 LAN 网卡名正确且 SharedAccess 服务可启动）", err)
	}
	logger.Info("windows: ICS 已启用 public=%s private=%s（VPN→LAN NAT 回退）", lanIf, tunName)

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

func teardownNATPlatform(vpnSubnet, lanCIDR, tunName string) error {
	_ = vpnSubnet
	_ = lanCIDR
	_ = tunName
	out, err := platform.Command("powershell", "-NoProfile", "-Command",
		`Remove-NetNat -Name `+brand.WinNATName+` -Confirm:$false -ErrorAction SilentlyContinue`).CombinedOutput()
	if err != nil {
		logger.Debug("Remove-NetNat: %s %v", out, err)
	}
	// 尽力关闭 ICS，避免残留影响其它网络共享
	icsOff := `
$ErrorActionPreference = 'SilentlyContinue'
regsvr32 /s hnetcfg.dll
$net = New-Object -ComObject HNetCfg.HNetShare
foreach ($c in @($net.EnumEveryConnection())) {
  try { $net.INetSharingConfigurationForINetConnection($c).DisableSharing() } catch {}
}
`
	_ = platform.Command("powershell", "-NoProfile", "-Command", icsOff).Run()
	return nil
}

// addClientRoutePlatform 添加分流路由：经 Wintun 接口 on-link（忽略 gateway 作下一跳）。
func addClientRoutePlatform(cidr, tunName, gateway string) error {
	_ = gateway
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	ifIndex, err := winnet.InterfaceIndex(tunName)
	if err != nil {
		return err
	}
	args := WindowsOnLinkRouteArgs(dest, mask, ifIndex)
	cmd := platform.Command("route", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		// 已存在则视为成功（重连/重复应用策略）
		if strings.Contains(msg, "对象已存在") || strings.Contains(strings.ToLower(msg), "exists") {
			return nil
		}
		return platform.CommandOutputError("route "+strings.Join(args, " "), out, err)
	}
	return nil
}

func delClientRoutePlatform(cidr, tunName, gateway string) error {
	dest, mask, err := netutil.SplitCIDR(cidr)
	if err != nil {
		return err
	}
	_ = tunName
	_ = gateway
	cmd := platform.Command("route", "DELETE", dest, "MASK", mask)
	_ = cmd.Run()
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

// winNATMatches 检查是否已有相同 prefix 的 NetNat 规则（避免每次重启 Remove+New）。
func winNATMatches(name, prefix string) bool {
	ps := fmt.Sprintf(`
$n = Get-NetNat -Name '%s' -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $n) { exit 1 }
if ($n.InternalIPInterfaceAddressPrefix -eq '%s') { exit 0 }
exit 2
`, winnet.EscapeSingleQuoted(name), winnet.EscapeSingleQuoted(prefix))
	err := platform.Command("powershell", "-NoProfile", "-Command", ps).Run()
	return err == nil
}
