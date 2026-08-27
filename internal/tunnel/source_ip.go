package tunnel

import (
	"fmt"
	"net"
	"strings"
)

// CheckTunnelSourceIP 校验隧道连接来源 IP 是否在白名单内；allowed 为空表示不限制。
func CheckTunnelSourceIP(remoteAddr string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("无法解析远端地址: %s", remoteAddr)
	}
	for _, rule := range allowed {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(rule); err == nil {
			if n.Contains(ip) {
				return nil
			}
			continue
		}
		if single := net.ParseIP(rule); single != nil && single.Equal(ip) {
			return nil
		}
	}
	return fmt.Errorf("隧道来源 IP %s 不在 tunnel_allowed_source_ips 白名单内", ip)
}
