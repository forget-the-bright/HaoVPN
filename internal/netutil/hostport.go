package netutil

import (
	"net"
	"strings"
)

// SplitHostPortLoose 宽松解析 "host:port"（不处理 IPv6 方括号格式）。
//
// 参数：addr — 如 "203.0.113.1:8443"；从右向左找最后一个 ':' 分割。
// 返回：ok 为 false 表示无法解析；适用于导出 YAML、简单日志等非 net.SplitHostPort 场景。
func SplitHostPortLoose(addr string) (host, port string, ok bool) {
	if i := strings.LastIndex(addr, ":"); i > 0 && i < len(addr)-1 {
		return addr[:i], addr[i+1:], true
	}
	return "", "", false
}

// SplitRemoteAddr 从 remoteAddr 拆出主机（去方括号）与端口。
//
// 参数：remoteAddr — 如 "203.0.113.1:8443"、"[2001:db8::1]:443" 或裸 IP。
// 返回：ip 为去掉 [] 的主机；port 在无法解析时为空，ip 回落为 HostFromAddr(remoteAddr)。
// 用途：探针防御、握手拒绝审计、日志中的 client_ip/client_port；统一替代各包私有 SplitHostPort。
// 关联：HostFromAddr（只要主机）、SplitHostPortLoose（宽松无 IPv6）。
func SplitRemoteAddr(remoteAddr string) (ip, port string) {
	host, p, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return HostFromAddr(remoteAddr), ""
	}
	return strings.Trim(host, "[]"), p
}
