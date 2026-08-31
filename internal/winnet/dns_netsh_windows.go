//go:build windows

package winnet

import (
	"fmt"

	"haovpn/internal/platform"
)

// SetInterfaceDNSStatic 将接口主 DNS（index=1）设为静态地址。
//
// 参数：ifName — netsh 别名；server — 单个 IPv4 DNS。
// 关联：netstack ApplyDNS；解析列表见 ParseDNSShowOutput。
func SetInterfaceDNSStatic(ifName, server string) error {
	return RunNetsh("interface", "ipv4", "set", "dnsservers", ifName,
		"source=static", "address="+server, "register=none", "validate=no")
}

// AddInterfaceDNS 向接口追加次级 DNS 服务器。
//
// 参数：index — netsh DNS 优先级（通常从 2 起）；server — IPv4 地址。
func AddInterfaceDNS(ifName, server string, index int) error {
	return RunNetsh("interface", "ipv4", "add", "dnsservers", ifName, server,
		"index="+fmt.Sprintf("%d", index), "validate=no")
}

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
