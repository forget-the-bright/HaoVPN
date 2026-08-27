package config

import (
	"fmt"
	"net"
	"strings"
)

// validateSubnetGateway 校验 VPN 子网 CIDR 与网关 IP 合法性。
func validateSubnetGateway(subnet, gateway string) error {
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

// validateCIDRList 校验 CIDR/IP 列表（空列表合法）。
func validateCIDRList(field string, cidrs []string) error {
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
