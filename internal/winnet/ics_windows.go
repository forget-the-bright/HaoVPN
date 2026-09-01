//go:build windows

package winnet

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
)

// DisableAllICSContext 关闭本机全部 ICS 共享；ctx 取消时 Kill PowerShell（日志 ics_abort）。
func DisableAllICSContext(ctx context.Context) {
	start := time.Now()
	if err := safeutil.Check(ctx); err != nil {
		logger.Info("ics_abort stage=DisableAllICS_before err=%v elapsed=%s", err, time.Since(start))
		return
	}
	ps := "$ErrorActionPreference = 'SilentlyContinue'\n" + PSSnippetICSDisableAll()
	RunPSBestEffortContext(ctx, ps, "DisableAllICS")
	logger.Info("DisableAllICS elapsed=%s", time.Since(start))
}

// DisableICSPairContext 仅关闭本会话 public/private 两块网卡上的 ICS 共享；ctx 取消时 Kill。
func DisableICSPairContext(ctx context.Context, public, private string) {
	public = strings.TrimSpace(public)
	private = strings.TrimSpace(private)
	if public == "" && private == "" {
		return
	}
	start := time.Now()
	if err := safeutil.Check(ctx); err != nil {
		logger.Info("ics_abort stage=DisableICSPair_before err=%v elapsed=%s", err, time.Since(start))
		return
	}
	// 脚本模板唯一源：PSSnippetICSDisablePair（与 netstack 清共享片段同源风格）。
	ps := PSSnippetICSDisablePair(public, private)
	RunPSBestEffortContext(ctx, ps, "DisableICSPair")
	logger.Info("DisableICSPair elapsed=%s public=%s private=%s", time.Since(start), public, private)
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
		if !netutil.InterfaceNameLooksLikeTUN(iface.Name, configName) {
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
		if netutil.IPv4IsICSPrivate(ip) {
			return true
		}
	}
	return false
}

// hasICSResiduePS 极端回退：冷启 powershell 在公司机可能十余秒，仅 native 找不到网卡时使用。
func hasICSResiduePS(configName string) bool {
	ps := PSSnippetProbeICSResidue(configName)
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
	return CleanupICSResidueContext(context.Background(), configName, vpnIP)
}

// CleanupICSResidueContext 同 CleanupICSResidue，ctx 取消时 Kill（返回 ctx.Err()）。
func CleanupICSResidueContext(ctx context.Context, configName, vpnIP string) error {
	vpnIP = strings.TrimSpace(vpnIP)
	if configName == "" || vpnIP == "" {
		return fmt.Errorf("CleanupICSResidue: configName/vpnIP 为空")
	}
	start := time.Now()
	if err := safeutil.Check(ctx); err != nil {
		logger.Info("ics_abort stage=CleanupICSResidue_before err=%v elapsed=%s", err, time.Since(start))
		return err
	}
	ps := fmt.Sprintf(`
$ErrorActionPreference = 'SilentlyContinue'
%s
%s
`, PSSnippetICSDisableAll(), PSSnippetRemoveNonVPNKeepVPN(vpnIP, PSSnippetAssignAdapterIf(configName)))
	_, err := RunPSOneShotContext(ctx, ps)
	if safeutil.IsCanceled(err) {
		logger.Info("ics_abort stage=CleanupICSResidue err=%v elapsed=%s", err, time.Since(start))
		return err
	}
	logger.Info("CleanupICSResidue elapsed=%s tun=%s keep=%s err=%v", time.Since(start), configName, vpnIP, err)
	return err
}

// DisableICSSessionContext 本会话 ICS 智能关闭（Logout / 显式全清；HardRestart 走 TeardownKeepICS 勿调用）。
//
// 策略（与 clientapp cleanupTUNAfterViaDisabled 互补，勿混用）：
//   - 有 RememberICSPair：先 DisableICSPairContext（快）；
//   - Pair 后仍有 192.168.137.* → DisableAllICSContext 兜底；
//   - Pair 后无残留 → 绝不再跑 DisableAll（避免双倍 COM）；
//   - 无 Pair：直接 DisableAllICSContext。
//
// 参数：ctx — 取消时 Kill PowerShell；正常 Teardown 传 Background 以确保清完。
// 关联：netstack.disableICSPlatform；via Teardown。空 local_lans 残留清理仍走 CleanupICSResidue。
func DisableICSSessionContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	pub, prv, ok := TakeICSPair()
	if ok {
		DisableICSPairContext(ctx, pub, prv)
		if safeutil.Check(ctx) != nil {
			logger.Info("ics_abort stage=DisableICSSession_after_pair elapsed=%s", time.Since(start))
			return
		}
		tun := prv
		if tun == "" {
			tun = pub
		}
		if HasICSResidue(tun) {
			logger.Info("windows: DisableICSPair 后仍有 ICS 残留，DisableAllICS 兜底 tun=%s", tun)
			DisableAllICSContext(ctx)
		} else {
			logger.Info("windows: DisableICSPair 后无残留，跳过 DisableAllICS tun=%s", tun)
		}
	} else {
		DisableAllICSContext(ctx)
	}
	logger.Info("DisableICSSession elapsed=%s pair=%v", time.Since(start), ok)
}
