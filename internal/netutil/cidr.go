package netutil

import (
	"fmt"
	"net"
	"strings"
)

// MaskPrefixFromSubnet 从 CIDR 字符串提取 IPv4 前缀长度（掩码位数）。
//
// 参数：subnet — 如 "10.88.0.0/24"。
// 返回：前缀长度字符串如 "24"；解析失败时回退 "24"，避免下游 netsh/route 因空掩码失败。
func MaskPrefixFromSubnet(subnet string) string {
	_, n, err := net.ParseCIDR(subnet)
	if err != nil {
		return "24"
	}
	ones, _ := n.Mask.Size()
	return fmt.Sprintf("%d", ones)
}

// GatewayCIDR 构造「网关 IP/前缀长度」形式的 CIDR 字符串。
//
// 参数：gatewayIP — 如 "10.88.0.1"；subnet — VPN 子网，用于推导前缀长度。
// 返回：如 "10.88.0.1/24"；常用于 Windows route 或日志。
func GatewayCIDR(gatewayIP, subnet string) string {
	return gatewayIP + "/" + MaskPrefixFromSubnet(subnet)
}

// ValidateCIDRList 校验 CIDR 或单 IP 列表的合法性。
//
// 参数：
//   field — 字段名，写入错误信息（如 "allowed_ips"）。
//   cidrs — 空列表合法；空白项自动跳过。
// 返回：含无效项时 error。
func ValidateCIDRList(field string, cidrs []string) error {
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err == nil {
			continue
		}
		if net.ParseIP(c) == nil {
			return fmt.Errorf("%s 含无效项 %q", field, c)
		}
	}
	return nil
}

// ValidateSubnetGateway 校验 VPN 子网 CIDR 与网关 IP 的合法性及包含关系。
//
// 参数：subnet — yaml vpn.subnet；gateway — yaml vpn.gateway_ip。
// 返回：子网无效、网关无效或网关不在子网内时 error。
func ValidateSubnetGateway(subnet, gateway string) error {
	_, n, err := net.ParseCIDR(strings.TrimSpace(subnet))
	if err != nil {
		return fmt.Errorf("vpn.subnet 无效 CIDR %q: %w", subnet, err)
	}
	ip := net.ParseIP(strings.TrimSpace(gateway))
	if ip == nil {
		return fmt.Errorf("vpn.gateway_ip 无效: %q", gateway)
	}
	if !n.Contains(ip) {
		return fmt.Errorf("vpn.gateway_ip %s 不在 vpn.subnet %s 内", gateway, subnet)
	}
	return nil
}

// ValidateNoFullTunnel 禁止 AllowedIPs 中出现全隧道 0.0.0.0/0 或 ::/0。
//
// HaoVPN 定位为工控分流 VPN，不允许通过配置覆盖全部流量。
// 返回：含全隧道前缀或无效 CIDR 时 error。
// 关联：ForbidDefaultRoute（单条）；本函数额外校验每项可解析。
func ValidateNoFullTunnel(cidrs []string) error {
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if err := ForbidDefaultRoute(c); err != nil {
			return fmt.Errorf("禁止 0.0.0.0/0 全隧道")
		}
		if _, err := ParseCIDROrHost(c); err != nil {
			return fmt.Errorf("无效 CIDR %q: %w", c, err)
		}
	}
	return nil
}

// ValidateIPInSubnet 校验 IPv4 是否在子网内且未占用 reserved 地址（通常为网关）。
//
// 参数：
//   ipStr — 待分配或握手下发的 VPN IP。
//   subnet — VPN 地址池 CIDR。
//   reserved — 可选不可占用地址列表。
// 返回：无效 IP、占用网关或不在子网内时 error。
func ValidateIPInSubnet(ipStr, subnet string, reserved ...string) error {
	ipStr = strings.TrimSpace(ipStr)
	parsed := net.ParseIP(ipStr)
	if parsed == nil || parsed.To4() == nil {
		return fmt.Errorf("无效 VPN IP: %s", ipStr)
	}
	ipNorm := parsed.To4().String()
	for _, r := range reserved {
		r = strings.TrimSpace(r)
		if r != "" && ipNorm == r {
			return fmt.Errorf("不能占用网关 IP %s", r)
		}
	}
	_, sn, err := net.ParseCIDR(strings.TrimSpace(subnet))
	if err != nil {
		return fmt.Errorf("服务端 VPN 子网配置无效")
	}
	if !sn.Contains(parsed) {
		return fmt.Errorf("VPN IP %s 不在子网 %s 内", ipNorm, subnet)
	}
	return nil
}

// ParseCIDR 解析「地址/前缀」字符串为主机 IP 与 *net.IPNet。
//
// 参数：cidr — 标准 net.ParseCIDR 格式。
// 返回：主机 IP、网段；解析失败时 error 含原始 cidr。
// 用途：tun 配置、ippool、路由探测等统一入口，避免各包重复包装 net.ParseCIDR。
func ParseCIDR(cidr string) (net.IP, *net.IPNet, error) {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CIDR %q: %w", cidr, err)
	}
	return ip, n, nil
}

// SplitCIDR 将 IPv4 CIDR 拆成 Windows route 命令所需的目标网络与掩码字符串。
//
// 参数：cidr — 如 "10.88.0.0/24"。
// 返回：dest 为网络地址、mask 为点分掩码；非 IPv4 或解析失败时 error。
// 用途：netstack Windows 路由 ADD/DELETE；与 GatewayCIDR 互补（后者用于网关/on-link）。
func SplitCIDR(cidr string) (dest, mask string, err error) {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", err
	}
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return "", "", fmt.Errorf("仅支持 IPv4: %s", cidr)
	}
	_ = ones
	return ip.Mask(n.Mask).String(), net.IP(n.Mask).String(), nil
}

// ParseCIDRToV4Mask 将 IPv4 CIDR 解析为主机序 uint32 地址与掩码（WFP 过滤条件用）。
//
// 参数：cidr — 如 "192.168.1.0/24"。
// 返回：addr、mask 为大端 uint32；非 IPv4 或无效 CIDR 时 error。
// 用途：Windows WFP 杀开关 remoteip 条件；纯数学，无平台 shell 依赖。
func ParseCIDRToV4Mask(cidr string) (addr, mask uint32, err error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, 0, err
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, 0, fmt.Errorf("非 IPv4: %s", cidr)
	}
	m := ipnet.Mask
	if len(m) != 4 {
		return 0, 0, fmt.Errorf("无效掩码: %s", cidr)
	}
	addr = uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	mask = uint32(m[0])<<24 | uint32(m[1])<<16 | uint32(m[2])<<8 | uint32(m[3])
	return addr, mask, nil
}

