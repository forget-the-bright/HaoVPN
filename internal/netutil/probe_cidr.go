package netutil

// ProbeIPForCIDR 取 LAN 网段内用于路由/ICS 探测的代表性 IPv4（通常为 network+1）。
//
// 参数：lanCIDR — 工控网段 CIDR；解析失败时回退 192.168.1.1。
// 返回：可用于 Find-NetRoute 等探测的目标 IP 字符串。
// 关联：netstack ics_egress、winnet egress ResolveOutboundNatural。
func ProbeIPForCIDR(lanCIDR string) string {
	ip, ipnet, err := ParseCIDR(lanCIDR)
	if err != nil {
		return "192.168.1.1"
	}
	probe := ip.Mask(ipnet.Mask).To4()
	if probe == nil {
		return ip.String()
	}
	if probe[3] < 254 {
		probe[3]++
	}
	return probe.String()
}
