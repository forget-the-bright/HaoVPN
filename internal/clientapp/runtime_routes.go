package clientapp

import (
	"net"
	"strings"

	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
)

// runtime_routes.go：系统分流路由的安装、差分删除与清理。

// installRouteListLocked 按期望列表全量安装路由；返回成功添加条数。调用方须已持 rt.mu。
func (rt *runtime) installRouteListLocked(desired []string, gw, tunName string) int {
	rt.routes = nil
	n := 0
	for _, cidr := range desired {
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
			continue
		}
		rt.routes = append(rt.routes, cidr)
		n++
	}
	return n
}

// syncRoutesDiffLocked 对已装路由与期望做集合差分；返回 add/del 数量。调用方须已持 rt.mu。
func (rt *runtime) syncRoutesDiffLocked(desired []string, gw, tunName string) (addN, delN int) {
	add, del := routeSetDiff(rt.routes, desired)
	delSet := make(map[string]struct{}, len(del))
	for _, c := range del {
		delSet[c] = struct{}{}
		if err := netstack.DelClientRoute(c, tunName, gw); err != nil {
			logger.Warn("删除路由 %s: %v", c, err)
		}
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
			continue
		}
		keep = append(keep, c)
	}
	rt.routes = normalizeRouteList(keep)
	return len(add), len(del)
}

// gatewayHostRouteNeeded 判断是否需单独添加网关主机路由（/32）。
// 若 AllowedIPs 中已有 CIDR 包含网关 IP（如 10.88.0.0/24 含 10.88.0.1），则不必再装。
func gatewayHostRouteNeeded(gw string, allowed []string) bool {
	ip := net.ParseIP(strings.TrimSpace(gw))
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
	// 仅当本会话曾 Setup via（rt.via != nil）时 Teardown 才会 DisableAllICS；未开 via 则跳过慢速 COM。
	rt.teardownViaExitLocked()
	rt.viaFP = ""
	rt.viaFPKnown = false
	rt.clearRoutesOnlyLocked()
	rt.appliedDNS = nil
	if rt.tunDev == nil {
		return
	}
	_ = netstack.RestoreDNS(rt.tunDev.Name())
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
		_ = netstack.DelClientRoute(cidr, tunName, gw)
	}
	rt.routes = nil
}
