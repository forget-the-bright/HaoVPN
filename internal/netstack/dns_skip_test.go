//go:build windows

package netstack

import "testing"

// TestFreshTUNDNSSnapshotSemantics 钉死新 TUN 首次快照为 dhcp=true、无服务器（skip_empty）。
func TestFreshTUNDNSSnapshotSemantics(t *testing.T) {
	ClearDNSSavedForTest()
	NoteSavedDNSForTest("haovpn_client", true, nil)
	dhcp, servers, ok := TakeDNSSavedForTest("haovpn_client")
	if !ok || !dhcp || len(servers) != 0 {
		t.Fatalf("dhcp=%v servers=%v ok=%v", dhcp, servers, ok)
	}
}
