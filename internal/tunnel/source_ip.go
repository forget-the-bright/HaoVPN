package tunnel

import (
	"fmt"

	"haovpn/internal/netutil"
)

// CheckTunnelSourceIP 校验隧道连接来源 IP 是否在白名单内；allowed 为空表示不限制。
//
// 参数：
//   remoteAddr — transport.Conn.RemoteAddr()，可为 host:port。
//   allowed — tunnel_allowed_source_ips 配置；支持 CIDR 与单 IP。
// 返回：不在白名单或地址无法解析时 error。
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
	return fmt.Errorf("隧道来源 IP %s 不在 tunnel_allowed_source_ips 白名单内", ip)
}
