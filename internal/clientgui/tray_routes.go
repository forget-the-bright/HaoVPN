package clientgui

import (
	"fmt"
	"strings"

	"haovpn/internal/tunnel"
)

// trayRouteLines 托盘「本机路由」子菜单文案（纯函数，便于单测）。
//
// 结构：
//  1. 本机 TUN（vpn_subnet via 网关）
//  2. 「—— 分流 ——」+ AllowedIPs（排除已展示的 VPN 子网）
//  3. 「—— 对端托管 ——」+ managed_routes；空则提示无对端托管
func trayRouteLines(vpnSubnet, vpnIP, gateway string, allowedIPs []string, managed []tunnel.ManagedRoute) []string {
	var lines []string

	tun := formatTUNLine(vpnSubnet, vpnIP, gateway)
	lines = append(lines, tun)

	lines = append(lines, "—— 分流 ——")
	subnetKey := normalizeCIDRKey(vpnSubnet)
	if subnetKey == "" {
		subnetKey = normalizeCIDRKey(deriveVPNSubnetHint(vpnIP))
	}
	splitN := 0
	for _, cidr := range allowedIPs {
		c := strings.TrimSpace(cidr)
		if c == "" {
			continue
		}
		if subnetKey != "" && normalizeCIDRKey(c) == subnetKey {
			continue // VPN 子网已在本机 TUN 行展示
		}
		lines = append(lines, formatSplitRouteLine(c, gateway))
		splitN++
	}
	if splitN == 0 {
		lines = append(lines, "（无工控/分流前缀）")
	}

	lines = append(lines, "—— 对端托管 ——")
	if len(managed) == 0 {
		lines = append(lines, "（无对端托管路由）")
	} else {
		for _, mr := range managed {
			lines = append(lines, formatManagedRouteLine(mr))
		}
	}
	return lines
}

// formatTUNLine 本机 VPN 子网行；优先握手 vpn_subnet，否则由 VPN IP 推导 /24。
func formatTUNLine(vpnSubnet, vpnIP, gateway string) string {
	subnet := strings.TrimSpace(vpnSubnet)
	if subnet == "" {
		subnet = deriveVPNSubnetHint(vpnIP)
	}
	gw := strings.TrimSpace(gateway)
	if subnet != "" && gw != "" {
		return fmt.Sprintf("%s via %s (本机TUN)", subnet, gw)
	}
	if gw != "" {
		return fmt.Sprintf("网关 %s via 本机TUN", gw)
	}
	if subnet != "" {
		return subnet + " (本机TUN)"
	}
	return "VPN 本机"
}

// formatSplitRouteLine AllowedIPs/NAT 工控段展示行。
func formatSplitRouteLine(cidr, gateway string) string {
	gw := strings.TrimSpace(gateway)
	if gw != "" {
		return fmt.Sprintf("%s via %s", strings.TrimSpace(cidr), gw)
	}
	return strings.TrimSpace(cidr)
}

// formatManagedRouteLine 对端托管路由一行；Stale 标「失效」。
func formatManagedRouteLine(mr tunnel.ManagedRoute) string {
	dest := strings.TrimSpace(mr.Dest)
	via := strings.TrimSpace(mr.ViaIP)
	var base string
	if via != "" {
		base = fmt.Sprintf("%s via %s", dest, via)
	} else {
		name := strings.TrimSpace(mr.ViaUsername)
		if name == "" {
			name = "via"
		}
		base = fmt.Sprintf("%s via %s(离线)", dest, name)
	}
	if mr.Stale {
		return base + "（失效）"
	}
	return base
}

// deriveVPNSubnetHint 无 vpn_subnet 时由 IPv4 主机地址粗推 x.y.z.0/24（仅展示回退）。
func deriveVPNSubnetHint(vpnIP string) string {
	parts := strings.Split(strings.TrimSpace(vpnIP), ".")
	if len(parts) == 4 {
		return parts[0] + "." + parts[1] + "." + parts[2] + ".0/24"
	}
	return strings.TrimSpace(vpnIP)
}

// normalizeCIDRKey 比较用：去空白、小写。
func normalizeCIDRKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
