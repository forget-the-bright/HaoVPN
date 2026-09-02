package vpnaccount

import (
	"strings"

	"haovpn/internal/logger"
)

// ResolveDNSForUser 按托管 DNS 表解析应对该账号下发的 DNS 列表（members−excludes）。
//
// 参数：userID — 握手账号；gatewayFallback — 无命中时回落（通常为 VPN 网关，与旧空 dns_servers 行为一致）。
// 软 DNS：仅写入握手 dns_servers，不再 MergeDNSIntoAllowedIPs。
func (s *Service) ResolveDNSForUser(userID int64, gatewayFallback string) []string {
	if s == nil || s.Store == nil || userID <= 0 {
		return fallbackDNS(gatewayFallback, s)
	}
	ips, err := s.Store.ListDNSIPsForUser(userID)
	if err != nil {
		logger.Warn("ResolveDNSForUser user_id=%d: %v", userID, err)
		return fallbackDNS(gatewayFallback, s)
	}
	if len(ips) == 0 {
		return fallbackDNS(gatewayFallback, s)
	}
	return ips
}

// fallbackDNS 无托管命中时回落：优先显式 gateway，否则 Cfg.VPN.GatewayIP。
func fallbackDNS(gatewayFallback string, s *Service) []string {
	gw := strings.TrimSpace(gatewayFallback)
	if gw == "" && s != nil && s.Cfg != nil {
		gw = strings.TrimSpace(s.Cfg.VPN.GatewayIP)
	}
	if gw == "" {
		return nil
	}
	return []string{gw}
}
