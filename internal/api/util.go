package api

import "fmt"

// AppendTunListenHost 将 TUN IP 追加到管理口监听列表（去重）。
func AppendTunListenHost(hosts []string, tunIP string) []string {
	if tunIP == "" {
		return hosts
	}
	for _, h := range hosts {
		if h == tunIP {
			return hosts
		}
	}
	return append(hosts, tunIP)
}

// FormatListenAddrs 格式化监听地址列表用于日志。
func FormatListenAddrs(hosts []string, port int) string {
	var s string
	for i, h := range hosts {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s:%d", h, port)
	}
	return s
}
