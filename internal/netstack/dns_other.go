//go:build !windows

package netstack

import "fmt"

// ApplyDNS 非 Windows：不支持 DNS 推送；有服务器列表时返回明确错误（不得假装已应用）。
func ApplyDNS(adapterName string, servers []string) error {
	if len(servers) == 0 {
		return nil
	}
	return fmt.Errorf("DNS 推送仅支持 Windows（adapter=%s servers=%v）", adapterName, servers)
}

// RestoreDNS 非 Windows 平台无操作。
func RestoreDNS(adapterName string) error {
	return nil
}

func DNSSavedCount() int                                          { return 0 }
func ClearDNSSavedForTest()                                       {}
func NoteSavedDNSForTest(adapter string, dhcp bool, servers []string) {}
func TakeDNSSavedForTest(adapter string) (bool, []string, bool)   { return false, nil, false }
