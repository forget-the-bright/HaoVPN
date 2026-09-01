package clientapp

import (
	"context"
	"fmt"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
)

// runtime_policy.go：按握手策略增量应用 TUN / 路由 / DNS / via。

// applyPolicy 按握手策略创建/更新 TUN、路由与 DNS（增量：仅改差异部分）。
//
// 参数：ctx — Engine.runCtx；Stop 时取消，须在 via/ICS 等慢阶段前检查以免空跑十余秒。
//
//	policy — 服务端 handshake_ok 下发的权威策略；VPNIP 非空。
//
// 返回：err 为 TUN 创建失败、via Setup 失败、ctx 取消，或期望分流路由全部安装失败（硬失败）；
//
//	部分路由失败不返回 err，文案写入 rt.routeWarn 供 Engine.LastError 展示。
//
// 副作用：可能关闭并重建 TUN、增删系统路由、修改网卡 DNS、Setup/Teardown via。
// 并发：调用方须持 Engine 锁或单 goroutine 调用；内部持 rt.mu。
//
// 装路由顺序：若预判本次会跑 via/ICS Setup，则先不装分流路由（ICS 会冲掉），
// Setup 成功后再清一次并全量安装；避免「装路由 → ICS → 再装路由」的重复开销。
func (rt *runtime) applyPolicy(ctx context.Context, policy tunnel.HandshakePolicy) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	totalStart := time.Now()
	rt.routeWarn = ""

	abort := func(stage string) error {
		if err := safeutil.Check(ctx); err != nil {
			logger.Info("policy_apply aborted stage=%s err=%v elapsed=%s", stage, err, time.Since(totalStart))
			return err
		}
		return nil
	}
	if err := abort("start"); err != nil {
		return err
	}

	// --- 阶段 1：校验 vpn_ip ---
	if policy.VPNIP == "" {
		return fmt.Errorf("握手未下发 vpn_ip")
	}

	mtu := netutil.ResolveMTU(policy.MTU, rt.cfg.Tun.MTU)
	gw := netutil.ResolveGateway(policy.GatewayIP, policy.VPNIP)
	desired := desiredClientRoutes(gw, policy.AllowedIPs)
	deferRoutes := rt.willViaSetupLocked(policy.VPNSubnet, policy.VPNIP, rt.cfg.LocalLANs)

	ipChanged := rt.tunDev != nil && rt.vpnIP != policy.VPNIP
	needColdOpen := rt.tunDev == nil
	mode := "noop"
	adapterLabel := ""
	addN, failN, delN := 0, 0, 0

	// --- 阶段 2：冷开 TUN / 软换 VPN IP / 保 TUN 差分路由；路由可推迟到 via 之后 ---
	tunStageStart := time.Now()
	switch {
	case needColdOpen:
		logger.Info("tun_open reason=first_policy new=%s", policy.VPNIP)
		dev, err := tun.Open(tun.Config{
			Name: rt.cfg.Tun.Name,
			MTU:  mtu,
			CIDR: policy.VPNIP + "/32",
		})
		if err != nil {
			return fmt.Errorf("TUN 创建失败: %w", err)
		}
		rt.tunDev = dev
		rt.vpnIP = policy.VPNIP
		rt.routes = nil
		rt.viaFP = ""
		rt.viaFPKnown = false
		rt.appliedDNS = nil
		rt.gateway = gw
		mode = "tun_recreate"
		adapterLabel = adapterReuseLabel(dev)
		logger.Info("policy_apply stage=tun elapsed=%s mode=open adapter=%s", time.Since(tunStageStart), adapterLabel)
		routesStart := time.Now()
		if !deferRoutes {
			addN, failN = rt.installRouteListLocked(desired, gw, rt.tunDev.Name())
		} else {
			logger.Info("policy_apply defer_routes reason=via_setup_pending")
		}
		logger.Info("policy_apply stage=routes elapsed=%s add=%d fail=%d defer=%v", time.Since(routesStart), addN, failN, deferRoutes)

	case ipChanged:
		// 在线仅改 VPN IP：不关 TUN、不拆 via/ICS、不清 viaFP（指纹不含 tunIP）。
		// ICS 在场时必须保留主机 /24（冷启 PreferVPN ics_prefix_keep）；强制 /32 = 变相 prefix_fix，NAT 立刻死。
		// Soft 路径不 Restart SharedAccess（Restart 会冲掉 137）；只换地址 + SkipAsSource。
		oldIP := rt.vpnIP
		logger.Info("vpn_ip_replace_inplace old=%s new=%s dataplane_keep=true", oldIP, policy.VPNIP)
		if err := abort("before_vpn_ip_inplace"); err != nil {
			return err
		}
		tunName := rt.tunDev.Name()
		dnsPoisonStart := time.Now()
		if err := netstack.RestoreDNS(tunName, oldIP, policy.VPNIP); err != nil {
			logger.Warn("vpn_ip_inplace RestoreDNS: %v", err)
		} else {
			logger.Info("vpn_ip_inplace RestoreDNS ok elapsed=%s poison=[%s %s]", time.Since(dnsPoisonStart), oldIP, policy.VPNIP)
		}
		// poison 后仅当握手 DNS 列表变化时才强制重装（见阶段 4）
		if rt.cfg.Tun.DNSFromPolicyEnabled() && len(policy.DNSServers) > 0 && !dnsServersEqual(rt.appliedDNS, policy.DNSServers) {
			rt.appliedDNS = nil
		}
		prefixLen := 32
		if netstack.HasICSResidue(tunName) {
			prefixLen = 24
			logger.Info("vpn_ip_inplace keep_ics_prefix=24 reason=has_137")
		}
		removed, kept, err := netstack.ReplaceTUNIPv4KeepICS(tunName, policy.VPNIP, prefixLen)
		if err != nil {
			return fmt.Errorf("vpn_ip_inplace ReplaceTUNIPv4KeepICS: %w", err)
		}
		logger.Info("vpn_ip_inplace replace removed=%v kept=%s/%d", removed, kept, prefixLen)
		if err := netstack.PreferVPNAfterSoftIPReplace(ctx, tunName, policy.VPNIP); err != nil {
			logger.Warn("vpn_ip_inplace PreferVPN light: %v", err)
		}
		oldGw := rt.gateway
		if oldGw == "" {
			oldGw = netutil.ResolveGateway("", oldIP)
		}
		rt.vpnIP = policy.VPNIP
		rt.gateway = gw
		mode = "vpn_ip_inplace"
		logger.Info("policy_apply stage=tun elapsed=%s mode=vpn_ip_inplace", time.Since(tunStageStart))
		routesStart := time.Now()
		if !deferRoutes {
			if oldGw == gw && routeListsEqual(rt.routes, desired) {
				logger.Info("vpn_ip_inplace routes=keep count=%d", len(rt.routes))
			} else {
				for _, cidr := range rt.routes {
					if e := netstack.DelClientRoute(cidr, tunName, oldGw); e != nil {
						logger.Warn("vpn_ip_inplace 删旧路由 %s: %v", cidr, e)
					}
				}
				delN = len(rt.routes)
				rt.routes = nil
				addN, failN = rt.installRouteListLocked(desired, gw, tunName)
			}
		} else {
			logger.Info("policy_apply defer_routes reason=via_setup_pending after_vpn_ip_inplace")
		}
		logger.Info("policy_apply stage=routes elapsed=%s add=%d fail=%d del=%d defer=%v", time.Since(routesStart), addN, failN, delN, deferRoutes)

	default:
		logger.Info("policy_apply stage=tun elapsed=%s mode=keep", time.Since(tunStageStart))
		tunName := rt.tunDev.Name()
		oldGw := rt.gateway
		if oldGw == "" {
			oldGw = netutil.ResolveGateway("", rt.vpnIP)
		}
		gwChanged := oldGw != gw
		routesStart := time.Now()
		if gwChanged {
			for _, cidr := range rt.routes {
				if err := netstack.DelClientRoute(cidr, tunName, oldGw); err != nil {
					logger.Warn("删除路由失败 cidr=%s gw=%s: %v", cidr, oldGw, err)
				}
			}
			delN = len(rt.routes)
			rt.routes = nil
			rt.gateway = gw
			mode = "routes_diff"
			if !deferRoutes {
				addN, failN = rt.installRouteListLocked(desired, gw, tunName)
			} else {
				logger.Info("policy_apply defer_routes reason=via_setup_pending after_gw_change")
			}
		} else {
			rt.gateway = gw
			if !deferRoutes {
				a, f, d := rt.syncRoutesDiffLocked(desired, gw, tunName)
				addN, failN, delN = a, f, d
				if addN > 0 || delN > 0 || failN > 0 {
					mode = "routes_diff"
				}
			} else {
				logger.Info("policy_apply defer_routes reason=via_setup_pending keep_existing_until_ics")
			}
		}
		logger.Info("policy_apply stage=routes elapsed=%s add=%d fail=%d del=%d defer=%v", time.Since(routesStart), addN, failN, delN, deferRoutes)
	}

	tunName := rt.tunDev.Name()

	// --- 阶段 3：更新策略版本与杀开关前缀 ---
	if policy.PolicyVer != rt.policyVer && rt.policyVer > 0 {
		logger.Info("策略已更新 policy_ver %d -> %d", rt.policyVer, policy.PolicyVer)
	}
	rt.policyVer = policy.PolicyVer
	rt.allowedCIDRs = append([]string{}, policy.AllowedIPs...)
	rt.cacheAllowedNetsLocked()

	// --- 阶段 4：按配置应用 DNS（列表未变则跳过）---
	// ICS 前立刻 ApplyDNS（与 defer_routes 独立）。
	if err := abort("before_dns"); err != nil {
		return err
	}
	dnsStart := time.Now()
	dnsChanged := false
	if rt.cfg.Tun.DNSFromPolicyEnabled() && len(policy.DNSServers) > 0 {
		if !dnsServersEqual(rt.appliedDNS, policy.DNSServers) {
			if err := netstack.ApplyDNS(tunName, policy.DNSServers); err != nil {
				logger.Warn("DNS 设置失败（未应用）adapter=%s: %v", tunName, err)
			} else {
				rt.appliedDNS = append([]string{}, policy.DNSServers...)
				dnsChanged = true
				logger.Info("dns_applied servers=%v adapter=%s", policy.DNSServers, tunName)
			}
		}
	}
	logger.Info("policy_apply stage=dns elapsed=%s changed=%v", time.Since(dnsStart), dnsChanged)

	// --- 阶段 5：via 出口；Setup 成功后再装/重装分流路由 ---
	// Stop/HardRestart 最常见卡点：ICS 十余秒；须在 Setup 前尊重 cancel
	if err := abort("before_via"); err != nil {
		return err
	}
	viaStart := time.Now()
	rt.cacheExitLANNetsLocked()
	viaDidSetup, err := rt.setupViaExitLocked(ctx, policy.VPNSubnet, tunName, policy.VPNIP, rt.cfg.LocalLANs)
	logger.Info("policy_apply stage=via_cleanup elapsed=%s did_setup=%v err=%v", time.Since(viaStart), viaDidSetup, err)
	if err != nil {
		// via 失败：若曾推迟装路由，补装一次以便排障/短暂可用，随后 dataplaneFailed 仍会全清
		if deferRoutes && len(rt.routes) == 0 && rt.tunDev != nil {
			addN, failN = rt.installRouteListLocked(desired, gw, tunName)
			logger.Warn("policy_apply via_fail_install_deferred_routes add=%d fail=%d", addN, failN)
		}
		return err
	}
	if viaDidSetup {
		rt.clearRoutesOnlyLocked()
		addN, failN = rt.installRouteListLocked(desired, gw, tunName)
		delN = 0
		if mode == "noop" {
			mode = "via_rebuild"
		}
		logger.Info("via_exit 后已安装客户端分流路由（仅此一次）")
		// PreferVPN 仅 ICS Setup 内一次；勿在装路由后再调。
	} else if deferRoutes {
		// 预判与实际不一致（极少）：补做差分/安装
		logger.Warn("policy_apply defer_routes mismatch，补装路由")
		a, f, d := rt.syncRoutesDiffLocked(desired, gw, tunName)
		addN, failN, delN = a, f, d
		if addN > 0 || delN > 0 || failN > 0 {
			mode = "routes_diff"
		}
	}
	// 纵深：仅本次刚跑 ICS Setup 时快路径 scrub（PreferVPN iphlp 已 scrub；此处多为 skip noop）
	if viaDidSetup {
		netstack.ScrubTUNDefaultRouteFast(tunName)
	}

	// 最终路由结果：零成功硬失败；部分失败写入 routeWarn（不阻断连接）
	okForCheck := addN
	if okForCheck == 0 && len(rt.routes) > 0 && failN == 0 {
		okForCheck = len(rt.routes)
	}
	warn, routeErr := checkRouteInstallResult(desired, okForCheck, failN)
	if routeErr != nil {
		return routeErr
	}
	if warn != "" {
		rt.routeWarn = warn
		logger.Warn("partial_routes=true %s", warn)
	}

	if mode == "noop" && dnsChanged {
		mode = "dns_only"
	}

	if adapterLabel != "" {
		logger.Info("policy_apply mode=%s open=session adapter=%s vpn_ip=%s add=%d fail=%d del=%d defer_routes=%v policy_ver=%d gateway=%s mtu=%d allowed_ips=%v total_elapsed=%s",
			mode, adapterLabel, policy.VPNIP, addN, failN, delN, deferRoutes, policy.PolicyVer, gw, mtu, policy.AllowedIPs, time.Since(totalStart))
	} else {
		logger.Info("policy_apply mode=%s vpn_ip=%s add=%d fail=%d del=%d defer_routes=%v policy_ver=%d gateway=%s mtu=%d allowed_ips=%v total_elapsed=%s",
			mode, policy.VPNIP, addN, failN, delN, deferRoutes, policy.PolicyVer, gw, mtu, policy.AllowedIPs, time.Since(totalStart))
	}
	return nil
}

// takeRouteWarn 取出并清空部分路由失败提示（供 Engine 写入 LastError）。
func (rt *runtime) takeRouteWarn() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	w := rt.routeWarn
	rt.routeWarn = ""
	return w
}

// takeICSHint 取出并清空 ICS 多网卡提示（确定走 ICS 后由 via Setup 写入）。
func (rt *runtime) takeICSHint() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	h := rt.icsHint
	rt.icsHint = ""
	return h
}

// adapterReuseHint 可选：Windows Wintun 设备报告是否复用已有适配器。
type adapterReuseHint interface {
	AdapterReused() bool
}

// adapterReuseLabel 返回 reuse|create|unknown，供 policy_apply 日志避免「mode=recreate」被误解为再建网卡。
func adapterReuseLabel(dev tun.Device) string {
	if h, ok := dev.(adapterReuseHint); ok {
		if h.AdapterReused() {
			return "reuse"
		}
		return "create"
	}
	return "unknown"
}
