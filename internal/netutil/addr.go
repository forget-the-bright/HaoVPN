package netutil

import (
	"fmt"
	"net"
	"strings"
)

// HostFromAddr 从 "host:port" 或裸 IP/主机名中提取主机部分。
//
// 参数：addr — 如 "203.0.113.1:8443"、"[2001:db8::1]:443" 或 "10.0.0.1"。
// 返回：去掉方括号的主机字符串；SplitHostPort 失败时原样返回 addr。
// 用途：TLS ServerName 推断、隧道来源 IP 日志、白名单前的 host 提取。
func HostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return strings.Trim(host, "[]")
}

// ParseHostIP 从地址字符串解析 IP（支持 host:port 与裸 IP）。
//
// 参数：addr — 远端地址或配置中的 server.address。
// 返回：解析成功的 net.IP；无法解析时 error。
// 用途：隧道来源白名单、审计日志中的客户端 IP。
func ParseHostIP(addr string) (net.IP, error) {
	host := HostFromAddr(addr)
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("无法解析远端地址: %s", addr)
	}
	return ip, nil
}

// NormalizeIPv4 将 IPv4 字符串规范化为点分十进制形式。
//
// 参数：ip — 如 "10.88.0.2" 或带前导零的变体。
// 返回：规范化的 IPv4 字符串；非 IPv4 时 error。
// 用途：VPN IP 入库、IP 池键、API 手动指定 IP 校验后统一格式。
func NormalizeIPv4(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return "", fmt.Errorf("无效 IPv4: %s", ip)
	}
	return parsed.To4().String(), nil
}

// DedupTrimNonEmpty 对字符串列表去首尾空白、去空项、保序去重。
//
// 参数：items — 如 AllowedIPs 前缀、DNS 列表；原切片不被修改。
// 返回：新切片；空输入返回 nil 或空切片均可接受。
// 用途：杀开关 WFP 前缀、配置项规范化。
func DedupTrimNonEmpty(items []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
