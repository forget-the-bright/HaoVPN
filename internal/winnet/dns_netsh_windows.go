//go:build windows

package winnet

import (
	"haovpn/internal/platform"
)

// 本文件保留 netsh 只读/恢复辅助；写 DNS 见 dns_iphlp_windows.go（iphlp→netsh）。

// RestoreInterfaceDNSDHCP 将接口 DNS 恢复为 DHCP 自动获取。
func RestoreInterfaceDNSDHCP(ifName string) error {
	return RunNetsh("interface", "ipv4", "set", "dnsservers", ifName, "source=dhcp")
}

// ShowInterfaceDNS 读取 netsh interface ipv4 show dnsservers 的原始输出。
//
// 返回：stdout 字节；netsh 失败时 error 与部分输出一并返回。
// 解析：调用方用 ParseDNSShowOutput 提取 IPv4 列表。
func ShowInterfaceDNS(ifName string) ([]byte, error) {
	out, err := platform.Command("netsh", "interface", "ipv4", "show", "dnsservers", ifName).CombinedOutput()
	if err != nil {
		return out, err
	}
	return out, nil
}
