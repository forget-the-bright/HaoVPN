package clientgui

import (
	"fmt"
	"strings"

	"haovpn/internal/clientapp"
	"haovpn/internal/netutil"
)

// trayRouteLines 托盘「本机路由」子菜单文案（纯函数，便于单测）。
func trayRouteLines(vpnSubnet, vpnIP, gateway string, allowedIPs []string, managed []clientapp.ManagedRouteView) []string {
	var lines []string

	tun := formatTUNLine(vpnSubnet, vpnIP, gateway)
	lines = append(lines, tun)

	lines = append(lines, "—— 分流 ——")
	subnetKey := netutil.TrimLower(vpnSubnet)
	if subnetKey == "" {
		subnetKey = netutil.TrimLower(netutil.InferVPNSubnetHint(vpnIP))
	}
	splitN := 0
	for _, cidr := range allowedIPs {
		c := strings.TrimSpace(cidr)
		if c == "" {
			continue
		}
		if subnetKey != "" && netutil.TrimLower(c) == subnetKey {
			continue
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

// formatManagedRouteLine 对端托管路由一行；Stale 标「失效」。
func formatManagedRouteLine(mr clientapp.ManagedRouteView) string {
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

// formatTUNLine 本机 VPN 子网行；优先握手 vpn_subnet，否则由 VPN IP 推导 /24。
func formatTUNLine(vpnSubnet, vpnIP, gateway string) string {
	subnet := strings.TrimSpace(vpnSubnet)
	if subnet == "" {
		subnet = netutil.InferVPNSubnetHint(vpnIP)
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

