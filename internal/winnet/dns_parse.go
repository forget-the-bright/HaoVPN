package winnet

import (
	"net"
	"strings"
)

// ParseDNSShowOutput 从 netsh interface ipv4 show dnsservers 输出中提取 IPv4 DNS 服务器列表。
//
// 参数：out — ShowInterfaceDNS / netsh 标准输出字节。
// 返回：按出现顺序的 IPv4 地址字符串；无法解析时可能为空切片。
//
// 为何放在 winnet：与 ShowInterfaceDNS 同属「Windows DNS 查询」域；纯解析、无系统调用，
// 故无 build tag，Linux/macOS 单测也可直接测。关联：netstack/dns_windows.go 的 readDNS。
func ParseDNSShowOutput(out []byte) []string {
	var servers []string
	for _, line := range strings.Split(string(out), "\n") {
		for _, p := range strings.Fields(strings.TrimSpace(line)) {
			if strings.Count(p, ".") == 3 && net.ParseIP(p) != nil {
				servers = append(servers, p)
			}
		}
	}
	return servers
}
