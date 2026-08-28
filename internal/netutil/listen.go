package netutil

import (
	"fmt"
	"net"
	"strings"
)

// IsWildcardListenHost 判断 host 是否为「监听全部网卡」的通配地址。
//
// 识别：0.0.0.0、::、[::]（去空白后）。
func IsWildcardListenHost(host string) bool {
	switch strings.TrimSpace(host) {
	case "0.0.0.0", "::", "[::]":
		return true
	default:
		return false
	}
}

// HasWildcardListenHost 判断监听地址列表是否已含通配绑定。
func HasWildcardListenHost(hosts []string) bool {
	for _, h := range hosts {
		if IsWildcardListenHost(h) {
			return true
		}
	}
	return false
}

// ValidatePublicBindPolicy 校验管理口公网绑定策略。
//
// 参数：hosts 含 0.0.0.0/:: 时 allowPublic 必须为 true，否则返回 error。
// 用途：server Validate 与 api 启动前二次防护。
func ValidatePublicBindPolicy(hosts []string, allowPublic bool) error {
	if !HasWildcardListenHost(hosts) {
		return nil
	}
	if !allowPublic {
		return fmt.Errorf("listen_hosts contains 0.0.0.0/:: but api.allow_public_bind is false; set allow_public_bind: true if you accept the risk")
	}
	return nil
}

// AppendTunListenHost 将 TUN 分配的 VPN IP 追加到管理口 listen_hosts（去重）。
//
// 若已配置通配地址则不再追加——通配已覆盖 VPN 网卡，再绑会端口冲突。
// 参数：tunIP 为空时原样返回 hosts。
func AppendTunListenHost(hosts []string, tunIP string) []string {
	if tunIP == "" || HasWildcardListenHost(hosts) {
		return hosts
	}
	for _, h := range hosts {
		if h == tunIP {
			return hosts
		}
	}
	return append(hosts, tunIP)
}

// ResolveListenAddrs 将 listen_hosts 与端口拼接为 net.Listen 可用的地址列表。
//
// 参数：hosts 为空时默认 ["127.0.0.1"]。
// 返回：经 net.JoinHostPort 规范化后的地址字符串切片。
func ResolveListenAddrs(hosts []string, port int) ([]string, error) {
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}
	var addrs []string
	for _, h := range hosts {
		addrs = append(addrs, net.JoinHostPort(strings.TrimSpace(h), fmt.Sprintf("%d", port)))
	}
	return addrs, nil
}
