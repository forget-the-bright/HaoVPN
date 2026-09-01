//go:build windows

package winnet

import (
	"time"

	"haovpn/internal/logger"
)

// RestoreInterfaceDNSDHCP 将接口 DNS 恢复为 DHCP 获取（Disconnect/Teardown 用）。
//
// 参数：ifName — netsh 接口别名（经 ResolveInterfaceAlias 解析后的名）。
func RestoreInterfaceDNSDHCP(ifName string) error {
	start := time.Now()
	err := RunNetsh("interface", "ipv4", "set", "dnsservers", ifName, "source=dhcp")
	logger.Info("dns_restore method=netsh_dhcp elapsed=%s ifName=%s err=%v", time.Since(start), ifName, err)
	return err
}
