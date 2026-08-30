package clientapp

import (
	"net"
	"sort"
	"strings"

	"haovpn/internal/netutil"
)

// normalizeRouteCIDR 将路由目标规范为可比对的 IPv4 CIDR 字符串。
// 单 IP 变为 /32；无效输入返回空串（调用方应跳过）。
func normalizeRouteCIDR(cidr string) string {
	s, err := netutil.NormalizeCIDROrHost(strings.TrimSpace(cidr))
	if err != nil {
		return ""
	}
	return s
}

// normalizeRouteList 规范化并去重，保持稳定排序便于日志与测试。
// 委托 netutil.NormalizeCIDRList，避免与 vpnaccount 策略合并各写一套。
func normalizeRouteList(cidrs []string) []string {
	return netutil.NormalizeCIDRList(cidrs, netutil.NormalizeCIDRListOpts{Sort: true})
}

// desiredClientRoutes 根据网关与 AllowedIPs 计算客户端应安装的路由集（已规范化排序）。
// 含按需网关 /32；若 AllowedIPs 已覆盖网关则不重复添加。
func desiredClientRoutes(gw string, allowed []string) []string {
	gw = strings.TrimSpace(gw)
	raw := make([]string, 0, len(allowed)+1)
	if gw != "" && gatewayHostRouteNeeded(gw, allowed) {
		raw = append(raw, gw+"/32")
	}
	for _, c := range allowed {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// 与 installClientRoutesLocked 一致：跳过与网关 /32 重复项
		if gw != "" && (c == gw+"/32" || normalizeRouteCIDR(c) == normalizeRouteCIDR(gw+"/32")) {
			continue
		}
		raw = append(raw, c)
	}
	return normalizeRouteList(raw)
}

// routeSetDiff 比较已装路由与期望路由，返回需新增与需删除的 CIDR（均已规范化）。
//
// 参数：
//   installed — 当前 rt.routes（可能未规范）；
//   desired — 期望集合（建议先经 desiredClientRoutes）。
func routeSetDiff(installed, desired []string) (add, del []string) {
	oldSet := make(map[string]struct{}, len(installed))
	for _, c := range normalizeRouteList(installed) {
		oldSet[c] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(desired))
	for _, c := range normalizeRouteList(desired) {
		newSet[c] = struct{}{}
	}
	for c := range newSet {
		if _, ok := oldSet[c]; !ok {
			add = append(add, c)
		}
	}
	for c := range oldSet {
		if _, ok := newSet[c]; !ok {
			del = append(del, c)
		}
	}
	sort.Strings(add)
	sort.Strings(del)
	return add, del
}

// viaFingerprint 标识 via/ICS 出口配置；相同则无需 teardown/Setup。
// lans 会经 ValidLANCIDRs；空 lans 返回空串表示 via 关闭。
func viaFingerprint(lans []string, vpnSubnet, tunIP string) string {
	valid := netutil.ValidLANCIDRs(lans)
	if len(valid) == 0 {
		return ""
	}
	sorted := append([]string{}, valid...)
	sort.Strings(sorted)
	subnet := strings.TrimSpace(vpnSubnet)
	ip := strings.TrimSpace(tunIP)
	if parsed := net.ParseIP(ip); parsed != nil {
		if v4 := parsed.To4(); v4 != nil {
			ip = v4.String()
		}
	}
	return strings.Join(sorted, ",") + "|" + subnet + "|" + ip
}

// dnsServersEqual 比较 DNS 列表（去空白后顺序敏感）；委托 netutil.StringSlicesEqualTrimmed。
func dnsServersEqual(a, b []string) bool {
	return netutil.StringSlicesEqualTrimmed(a, b)
}
