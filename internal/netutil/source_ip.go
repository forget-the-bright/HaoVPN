package netutil

import (
	"fmt"
	"net"

	"haovpn/internal/dialerr"
)

// CheckSourceIPAllowed 校验远端地址是否在 allowed 白名单内；空列表表示不限制。
//
// 参数 remoteAddr — host:port 或裸 IP；allowed — CIDR/单 IP 列表。
// 返回 nil 表示允许；不在白名单时 error 包装 dialerr.ErrSourceDenied（可 errors.Is）。
// 关联：probedefense.Guard.AllowSourceIP、tunnel.ServerHandler 握手阶段直接调用本函数。
func CheckSourceIPAllowed(remoteAddr string, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}
	ip, err := ParseHostIP(remoteAddr)
	if err != nil {
		return err
	}
	if IPMatchesRules(ip, allowed) {
		return nil
	}
	return fmt.Errorf("%w: %s", dialerr.ErrSourceDenied, ip)
}

// ParseHostIPOrNil 解析失败时返回 nil（网关路由等宽松场景）。
func ParseHostIPOrNil(addr string) net.IP {
	ip, err := ParseHostIP(addr)
	if err != nil {
		return nil
	}
	return ip
}
