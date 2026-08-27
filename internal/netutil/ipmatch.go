package netutil

import (
	"fmt"
	"net"
	"strings"
)

// ParseCIDROrHost 解析 CIDR 或单 IP 规则为 *net.IPNet。
//
// 参数：rule — 如 "192.168.1.0/24" 或 "10.0.0.5"；空串非法。
// 返回：单 IPv4 归一化为 /32，IPv6 为 /128；无法解析时 error。
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
