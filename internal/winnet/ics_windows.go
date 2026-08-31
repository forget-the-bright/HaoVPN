//go:build windows

package winnet

import (
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
)

// DisableAllICS 关闭本机全部 ICS 共享（via Teardown 或有残留清理时）。
//
// 通过 PowerShell COM 枚举连接并 DisableSharing，常见耗时数秒；调用方勿在 UI 线程同步执行。
// 空 local_lans 登录路径应先 HasICSResidue，无残留勿调用（见 CleanupICSResidue）。
// 本会话刚启用过 ICS 时优先 DisableICSPair（见 RememberICSPair），本函数作兜底。
func DisableAllICS() {
	start := time.Now()
	ps := "$ErrorActionPreference = 'SilentlyContinue'\n" + PSSnippetICSDisableAll()
	// 尽力关闭：COM 在部分环境会失败，不能阻断 Teardown；失败见 RunPSBestEffort 的 Warn。
	RunPSBestEffort(ps, "DisableAllICS")
	logger.Info("DisableAllICS elapsed=%s", time.Since(start))
}

// DisableICSPair 仅关闭本会话 public/private 两块网卡上的 ICS 共享（快于全机枚举逐个 Disable）。
//
// 参数：public — 出站侧（如 WLAN）；private — TUN 侧（如 haovpn_client）。
// 空参数时无操作。失败只打 Warn（BestEffort）。
func DisableICSPair(public, private string) {
	public = strings.TrimSpace(public)
	private = strings.TrimSpace(private)
	if public == "" && private == "" {
		return
	}
	start := time.Now()
	// 脚本模板唯一源：PSSnippetICSDisablePair（与 netstack 清共享片段同源风格）。
	ps := PSSnippetICSDisablePair(public, private)
	RunPSBestEffort(ps, "DisableICSPair")
	logger.Info("DisableICSPair elapsed=%s public=%s private=%s", time.Since(start), public, private)
}

// IPv4IsICSPrivate 判断是否为 ICS 默认私网地址（192.168.137.0/24）。
func IPv4IsICSPrivate(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4[0] == 192 && v4[1] == 168 && v4[2] == 137
}

// HasICSResidue 便宜探测 TUN/Wintun 上是否仍有 ICS 私网地址（192.168.137.*）。
//
// 参数：configName — TUN 配置名（如 haovpn0）；空则扫描已登记索引或按名匹配的网卡。
// 返回：true 表示存在 ICS 地址残留，值得跑慢速 CleanupICSResidue；探测失败当作无残留（false），
// 避免公司机无 via 时因探测异常反复触发十几秒 DisableAllICS。
// 实现：优先 Go/net + LUID 缓存（毫秒级）；仅在找不到网卡时才 PowerShell 回退并 Warn。
// 关联：clientapp/via_exit.go cleanupTUNAfterViaDisabled。
func HasICSResidue(configName string) bool {
	start := time.Now()
	hit, ok, stage := hasICSResidueNative(configName)
	if ok {
		logger.Info("HasICSResidue elapsed=%s method=native stage=%s hit=%v tun=%s", time.Since(start), stage, hit, configName)
		return hit
	}
	logger.Warn("HasICSResidue native 未找到网卡，回退 PowerShell tun=%s", configName)
	hit = hasICSResiduePS(configName)
	logger.Info("HasICSResidue elapsed=%s method=ps_fallback hit=%v tun=%s", time.Since(start), hit, configName)
	return hit
}

// hasICSResidueNative 用已登记 ifIndex / net.Interfaces 扫 137 地址。
// ok=false 表示未能定位目标网卡，调用方可回退 PS。
// 有 LUID 缓存时只查该 ifIndex，避免全量 net.Interfaces（公司机可数秒）。
func hasICSResidueNative(configName string) (hit bool, ok bool, stage string) {
	configName = strings.TrimSpace(configName)
	stageStart := time.Now()
	if idx, found := cachedIfIndex(configName); found && idx > 0 {
		hit = interfaceHasICSPrivateByIndex(idx)
		logger.Debug("HasICSResidue stage=cache elapsed=%s ifIndex=%d hit=%v", time.Since(stageStart), idx, hit)
		return hit, true, "cache"
	}
	stageStart = time.Now()
	if configName != "" {
		if iface, err := net.InterfaceByName(configName); err == nil {
			hit = interfaceAddrsHaveICSPrivate(iface)
			logger.Debug("HasICSResidue stage=by_name elapsed=%s hit=%v", time.Since(stageStart), hit)
			return hit, true, "by_name"
		}
		alias := ResolveInterfaceAlias(configName)
		if alias != "" && alias != configName {
			if iface, err := net.InterfaceByName(alias); err == nil {
				hit = interfaceAddrsHaveICSPrivate(iface)
				logger.Debug("HasICSResidue stage=by_alias elapsed=%s hit=%v", time.Since(stageStart), hit)
				return hit, true, "by_alias"
			}
		}
	}
	stageStart = time.Now()
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, false, "scan_ifaces"
	}
	matched := false
	for i := range ifaces {
		iface := &ifaces[i]
		if !ifaceNameLooksLikeTUN(iface.Name, configName) {
			continue
		}
		matched = true
		if interfaceAddrsHaveICSPrivate(iface) {
			logger.Debug("HasICSResidue stage=scan_ifaces elapsed=%s hit=true", time.Since(stageStart))
			return true, true, "scan_ifaces"
		}
	}
	logger.Debug("HasICSResidue stage=scan_ifaces elapsed=%s hit=false matched=%v", time.Since(stageStart), matched)
	return false, matched, "scan_ifaces"
}

func ifaceNameLooksLikeTUN(name, configName string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if configName != "" && (name == configName || strings.HasPrefix(name, configName+" ")) {
		return true
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, "wintun") || strings.Contains(lower, "haovpn")
}

func interfaceHasICSPrivateByIndex(ifIndex int) bool {
	if UseIPHelperEnabled() {
		idxStart := time.Now()
		hit, err := interfaceHasICSPrivateByIPHelper(ifIndex)
		if err == nil {
			logger.Debug("HasICSResidue by_index method=iphlp elapsed=%s ifIndex=%d hit=%v", time.Since(idxStart), ifIndex, hit)
			return hit
		}
		logger.Debug("HasICSResidue by_index method=net_fallback elapsed=%s ifIndex=%d err=%v", time.Since(idxStart), ifIndex, err)
	}
	idxStart := time.Now()
	iface, err := net.InterfaceByIndex(ifIndex)
	idxElapsed := time.Since(idxStart)
	if err != nil {
		logger.Debug("HasICSResidue by_index elapsed=%s ifIndex=%d err=%v", idxElapsed, ifIndex, err)
		return false
	}
	addrStart := time.Now()
	hit := interfaceAddrsHaveICSPrivate(iface)
	logger.Debug("HasICSResidue by_index method=net elapsed=%s addrs_elapsed=%s ifIndex=%d hit=%v", idxElapsed, time.Since(addrStart), ifIndex, hit)
	return hit
}

func interfaceAddrsHaveICSPrivate(iface *net.Interface) bool {
	if iface == nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if IPv4IsICSPrivate(ip) {
			return true
		}
	}
	return false
}

// hasICSResiduePS 极端回退：冷启 powershell 在公司机可能十余秒，仅 native 找不到网卡时使用。
func hasICSResiduePS(configName string) bool {
	ps := "$ErrorActionPreference = 'SilentlyContinue'\n" +
		PSSnippetAssignAdapterIf(configName) + `
if (-not $if) { Write-Output '0' }
else {
$hit = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -like '192.168.137.*' } | Select-Object -First 1
if ($hit) { Write-Output '1' } else { Write-Output '0' }
}
`
	out, err := RunPSOneShot(ps)
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
%s
$vpn = '%s'
%s
if (-not $if) { throw '未找到 Wintun 网卡' }
Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 |
  Where-Object { $_.IPAddress -ne $vpn -and $_.IPAddress -like '192.168.137.*' } |
  ForEach-Object { Remove-NetIPAddress -InterfaceIndex $_.InterfaceIndex -IPAddress $_.IPAddress -Confirm:$false -ErrorAction SilentlyContinue }
$has = Get-NetIPAddress -InterfaceIndex $if.ifIndex -AddressFamily IPv4 | Where-Object { $_.IPAddress -eq $vpn }
if (-not $has) {
  New-NetIPAddress -InterfaceIndex $if.ifIndex -IPAddress $vpn -PrefixLength 32 -ErrorAction SilentlyContinue | Out-Null
}
`, PSSnippetICSDisableAll(), EscapeSingleQuoted(vpnIP), PSSnippetAssignAdapterIf(configName))
	_, err := RunPSOneShot(ps)
	logger.Info("CleanupICSResidue elapsed=%s tun=%s keep=%s err=%v", time.Since(start), configName, vpnIP, err)
	return err
}
