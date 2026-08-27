//go:build !windows

package netstack

import "fmt"

// ApplyDNS 非 Windows 平台不支持 TUN 接口 DNS 推送。
//
// 参数：servers 非空时返回明确错误，避免假装已应用策略 DNS。
// 返回：len(servers)==0 时 nil；否则 error 说明仅 Windows 支持。
func ApplyDNS(adapterName string, servers []string) error {
	if len(servers) == 0 {
		return nil
	}
	return fmt.Errorf("DNS 推送仅支持 Windows（adapter=%s servers=%v）", adapterName, servers)
}

// RestoreDNS 非 Windows 平台无 DNS 快照机制，恒为无操作。
func RestoreDNS(adapterName string) error {
	return nil
}

// DNSSavedCount 非 Windows 恒为 0（无快照表）。
func DNSSavedCount() int { return 0 }

// ClearDNSSavedForTest 非 Windows 无操作（单测桩）。
func ClearDNSSavedForTest() {}

// NoteSavedDNSForTest 非 Windows 无操作（单测桩）。
func NoteSavedDNSForTest(adapter string, dhcp bool, servers []string) {}

// TakeDNSSavedForTest 非 Windows 恒返回 ok=false（单测桩）。
func TakeDNSSavedForTest(adapter string) (bool, []string, bool) { return false, nil, false }
