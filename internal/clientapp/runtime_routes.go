package clientapp

import (
	"fmt"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
)

// runtime_routes.go：系统分流路由的安装、差分删除与清理。

// installRouteListLocked 按期望列表全量安装路由。调用方须已持 rt.mu。
//
// 返回：okN 成功条数；failN 失败条数。
// 日志：route_install ok=N fail=M（排障检索）；单条失败 Warn 不中断循环。
// 调用方：若 desired 非空且 okN==0 须硬失败（见 checkRouteInstallResult）。
func (rt *runtime) installRouteListLocked(desired []string, gw, tunName string) (okN, failN int) {
	rt.routes = nil
	for _, cidr := range desired {
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
			failN++
			continue
		}
		rt.routes = append(rt.routes, cidr)
		okN++
	}
	logger.Info("route_install ok=%d fail=%d desired=%d", okN, failN, len(desired))
	return okN, failN
}

// checkRouteInstallResult 在装路由后判定：零成功硬失败；部分成功返回可展示警告文案。
//
// 参数：desired — 期望 CIDR 列表；okN/failN — install 或差分后统计。
// 返回：err 非 nil 表示期望非空却一条都没装上；warn 为部分失败时的用户可见提示（可空）。
func checkRouteInstallResult(desired []string, okN, failN int) (warn string, err error) {
	if len(desired) == 0 {
		return "", nil
	}
	if okN == 0 {
		return "", fmt.Errorf("分流路由全部安装失败（期望 %d 条，成功 0）；已拒绝进入已连接态以防流量泄漏或黑洞", len(desired))
	}
	if failN > 0 {
		return fmt.Sprintf("部分分流路由安装失败（成功 %d / 失败 %d）；工控网段可能不通，请查日志 route_install", okN, failN), nil
	}
	return "", nil
}

// syncRoutesDiffLocked 对已装路由与期望做集合差分。调用方须已持 rt.mu。
//
// 返回：addOk/addFail 新增成功/失败；delN 尝试删除条数（删除失败已 Warn）。
func (rt *runtime) syncRoutesDiffLocked(desired []string, gw, tunName string) (addOk, addFail, delN int) {
	add, del := routeSetDiff(rt.routes, desired)
	delSet := make(map[string]struct{}, len(del))
	for _, c := range del {
		delSet[c] = struct{}{}
		if err := netstack.DelClientRoute(c, tunName, gw); err != nil {
			logger.Warn("删除路由 %s: %v", c, err)
		}
		delN++
	}
	keep := make([]string, 0, len(rt.routes))
	for _, c := range normalizeRouteList(rt.routes) {
		if _, drop := delSet[c]; drop {
			continue
		}
		keep = append(keep, c)
	}
	for _, c := range add {
		if err := netstack.AddClientRoute(c, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", c, err)
			addFail++
			continue
		}
		keep = append(keep, c)
		addOk++
	}
	rt.routes = normalizeRouteList(keep)
	logger.Info("route_install ok=%d fail=%d desired=%d del=%d mode=diff", addOk, addFail, len(desired), delN)
	return addOk, addFail, delN
}

// gatewayHostRouteNeeded 判断是否需单独添加网关主机路由（/32）。
// 若 AllowedIPs 中已有 CIDR 包含网关 IP（如 10.88.0.0/24 含 10.88.0.1），则不必再装。
func gatewayHostRouteNeeded(gw string, allowed []string) bool {
	norm, err := netutil.NormalizeIPv4(gw)
	if err != nil {
		return false
	}
	ip := netutil.ParseHostIPOrNil(norm)
	if ip == nil {
		return false
	}
	return !netutil.CIDRListContainsIP(allowed, ip)
}

func (rt *runtime) clearRoutes() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
}

func (rt *runtime) clearRoutesLocked() {
	// Teardown：仅本会话曾 Setup via（rt.via != nil）时才会 DisableAllICS。
	// 空 local_lans 的慢清理不在这里：在 setupViaExitLocked → cleanupTUNAfterViaDisabled，
	// 且须先 HasICSResidue；无残留跳过（公司机常见路径）。
	//
	// DNS 须先于删路由：若先删路由再 RestoreDNS，系统可能仍指向 VPN DNS 却无回程，
	// 手动重连立刻 Dial 主机名会出现 lookup i/o timeout。
	rt.teardownViaExitLocked()
	rt.viaFP = ""
	rt.viaFPKnown = false
	if rt.tunDev != nil {
		dnsStart := time.Now()
		if err := netstack.RestoreDNS(rt.tunDev.Name()); err != nil {
			logger.Warn("RestoreDNS 失败 adapter=%s elapsed=%s: %v", rt.tunDev.Name(), time.Since(dnsStart), err)
		} else {
			logger.Info("RestoreDNS ok adapter=%s elapsed=%s", rt.tunDev.Name(), time.Since(dnsStart))
		}
	}
	rt.appliedDNS = nil
	rt.clearRoutesOnlyLocked()
}

func (rt *runtime) clearRoutesOnlyLocked() {
	if rt.tunDev == nil {
		return
	}
	tunName := rt.tunDev.Name()
	gw := rt.gateway
	if gw == "" {
		gw = netutil.ResolveGateway("", "", rt.vpnIP)
	}
	for _, cidr := range rt.routes {
		if err := netstack.DelClientRoute(cidr, tunName, gw); err != nil {
			// 禁止裸 _=：残留路由会导致下次连接分流异常，须可检索。
			logger.Warn("删除路由失败 cidr=%s: %v", cidr, err)
		}
	}
	rt.routes = nil
}
