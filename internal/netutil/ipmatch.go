package netutil

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// ParseCIDROrHost 解析 CIDR 或单 IP 规则为 *net.IPNet。
//
// 参数：rule — 如 "192.168.1.0/24" 或 "10.0.0.5"；空串非法。
// 返回：单 IPv4 归一化为 /32，IPv6 为 /128；无法解析时 error。
// 关联：NormalizeCIDROrHost（要规范化字符串）、CIDRListContainsIP。
func ParseCIDROrHost(rule string) (*net.IPNet, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return nil, fmt.Errorf("空规则")
	}
	if _, n, err := net.ParseCIDR(rule); err == nil {
		return n, nil
	}
	ip := net.ParseIP(rule)
	if ip == nil {
		return nil, fmt.Errorf("无效 CIDR 或 IP: %q", rule)
	}
	if ip.To4() != nil {
		return &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}

// NormalizeCIDROrHost 将 CIDR 或单 IP 规范为标准 CIDR 字符串。
//
// 参数：rule — 如 "10.88.0.5" → "10.88.0.5/32"；已是 CIDR 则返回 n.String()。
// 返回：规范化字符串；空/无法解析时 error。
// 用途：托管路由 dest、LAN 注册表、AllowedIPs 合并前统一格式，避免各包手写 ParseCIDR+ /32。
func NormalizeCIDROrHost(rule string) (string, error) {
	n, err := ParseCIDROrHost(rule)
	if err != nil {
		return "", err
	}
	return n.String(), nil
}

// ForbidDefaultRoute 拒绝全隧道默认路由 0.0.0.0/0 与 ::/0。
//
// 参数：cidr — 已规范化或原始 CIDR/单 IP；单 IP 不会触发拒绝。
// 返回：命中默认路由时 error（中文说明）；其它情况 nil。
// 关联：ValidateNoFullTunnel（列表版）、托管路由 / local_lans 入库校验。
func ForbidDefaultRoute(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "0.0.0.0/0" || cidr == "::/0" {
		return fmt.Errorf("禁止默认路由 %s", cidr)
	}
	n, err := ParseCIDROrHost(cidr)
	if err != nil {
		return nil // 解析交给调用方；本函数只拦默认路由
	}
	ones, bits := n.Mask.Size()
	if ones == 0 && (bits == 32 || bits == 128) {
		return fmt.Errorf("禁止默认路由 %s", n.String())
	}
	return nil
}

// CIDRListContainsIP 判断 ip 是否被 cidrs 中任一条 CIDR/单 IP 覆盖。
//
// 参数：无效项跳过；ip 为 nil 时 false。
// 用途：客户端判断是否还需装网关 /32；peer_policy 覆盖跳过。
func CIDRListContainsIP(cidrs []string, ip net.IP) bool {
	return IPMatchesRules(ip, cidrs)
}

// IPMatchesRules 判断 ip 是否命中 rules 中任一条 CIDR 或单 IP 规则。
//
// 参数：无效规则项被跳过；ip 为 nil 时恒 false。
func IPMatchesRules(ip net.IP, rules []string) bool {
	if ip == nil {
		return false
	}
	for _, rule := range rules {
		n, err := ParseCIDROrHost(rule)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IPNetsToStrings 将 []*net.IPNet 转为 CIDR 字符串列表（跳过 nil）。
//
// 参数：nets — 可为 nil。
// 返回：每项为 n.String()；用于 API JSON 响应与日志。
func IPNetsToStrings(nets []*net.IPNet) []string {
	var out []string
	for _, n := range nets {
		if n != nil {
			out = append(out, n.String())
		}
	}
	return out
}

// NormalizeCIDRListOpts 控制 NormalizeCIDRList 行为。
type NormalizeCIDRListOpts struct {
	// Sort 为 true 时对结果按字典序排序（便于路由 diff、via 指纹、单测稳定）。
	Sort bool
}

// NormalizeCIDRList 将 CIDR/单 IP 列表规范化并去重。
//
// 参数：cidrs — 原始列表；无效项跳过；空输入返回 nil。
// opts.Sort — 是否字典序排序（默认保序）。
// 用途：clientapp 路由差分、策略指纹；与 ValidLANCIDRs（广告 LAN 专用校验）互补。
func NormalizeCIDRList(cidrs []string, opts NormalizeCIDRListOpts) []string {
	seen := make(map[string]struct{}, len(cidrs))
	out := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		n, err := NormalizeCIDROrHost(strings.TrimSpace(c))
		if err != nil || n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	if opts.Sort {
		sort.Strings(out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// AppendCIDRUnique 解析并追加一条 CIDR 到列表（策略合并用）。
//
// 参数：
//   cidrs/nets — 已接受的前缀与对应 *IPNet（须同步维护；nets 可为 nil 且与 cidrs 等长）；
//   c — 待追加的 CIDR 或单 IP；
//   skipIfCovered — 为 true 时，若该网段网络地址已被 nets 中任一项 Contains 则跳过（peer/via /32 去冗余）。
// 返回：更新后的 cidrs、nets，以及是否真正追加。
// 关联：vpnaccount.ResolveClientPolicy。
func AppendCIDRUnique(cidrs []string, nets []*net.IPNet, c string, skipIfCovered bool) (out []string, outNets []*net.IPNet, added bool) {
	n, err := ParseCIDROrHost(c)
	if err != nil {
		return cidrs, nets, false
	}
	s := n.String()
	for _, exist := range cidrs {
		if exist == s {
			return cidrs, nets, false
		}
	}
	if skipIfCovered {
		ip := n.IP
		for _, exist := range nets {
			if exist != nil && exist.Contains(ip) {
				return cidrs, nets, false
			}
		}
	}
	return append(cidrs, s), append(nets, n), true
}

// ParseCIDRListToNets 将 allowed_ips 等字符串列表解析为 []*net.IPNet。
//
// 参数：cidrs — 无效项静默跳过。
// 返回：至少一条有效网段；全部无效或为空时 error（供 sessionmgr 注册前拒绝空策略）。
func ParseCIDRListToNets(cidrs []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		n, err := ParseCIDROrHost(cidr)
		if err != nil {
			continue
		}
		nets = append(nets, n)
	}
	if len(nets) == 0 {
		return nil, fmt.Errorf("AllowedIPs 为空或全部无效")
	}
	return nets, nil
}
