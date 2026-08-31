package netutil

import (
	"fmt"
	"net"
	"strings"
)

// ValidateIPOrCIDR 校验 s 为合法 IP；allowCIDR 为 true 时亦允许 CIDR。
//
// field 用于错误信息前缀（如 security.probe_defense.ban_exempt_ips）。
func ValidateIPOrCIDR(field, s string, allowCIDR bool) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("%s 不能为空", field)
	}
	if net.ParseIP(s) != nil {
		return nil
	}
	if !allowCIDR {
		return fmt.Errorf("%s 无效 IP: %s", field, s)
	}
	if _, _, err := net.ParseCIDR(s); err != nil {
		return fmt.Errorf("%s 无效 IP 或 CIDR: %s", field, s)
	}
	return nil
}
