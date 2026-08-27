package netutil

import "strings"

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
