package tunnel

import (
	"fmt"

	"haovpn/internal/autherr"
	"haovpn/internal/netutil"
)

// ErrSourceDenied 隧道来源不在 tunnel_allowed_source_ips；探针签名 source_deny。
var ErrSourceDenied = autherr.ErrSourceDenied

// CheckTunnelSourceIP 校验隧道连接来源 IP 是否在白名单内；allowed 为空表示不限制。
func CheckTunnelSourceIP(remoteAddr string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	ip, err := netutil.ParseHostIP(remoteAddr)
	if err != nil {
		return err
	}
	if netutil.IPMatchesRules(ip, allowed) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrSourceDenied, ip)
}
