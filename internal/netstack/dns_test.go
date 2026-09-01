package netstack

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestKillPrefixesDedupViaNetutil(t *testing.T) {
	// 杀开关前缀规范化已直接用 netutil.DedupTrimNonEmpty（无本包薄封装）。
	got := netutil.DedupTrimNonEmpty([]string{"192.168.1.0/24", "", "192.168.1.0/24", "10.0.0.0/8"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestDNSSavedTakeDoesNotRepollute(t *testing.T) {
	ClearDNSSavedForTest()
	NoteSavedDNSForTest("haovpn0", false, []string{"1.1.1.1"})
	dhcp, servers, ok := TakeDNSSavedForTest("haovpn0")
	if !ok || dhcp || len(servers) != 1 || servers[0] != "1.1.1.1" {
		t.Fatalf("take: dhcp=%v servers=%v ok=%v", dhcp, servers, ok)
	}
	if DNSSavedCount() != 0 {
		t.Fatal("snapshot must be cleared after take")
	}
	// 模拟错误路径：若再经 ApplyDNS 会把当前 VPN DNS 存回去——Take 后不应有存档
	NoteSavedDNSForTest("haovpn0", false, []string{"10.88.0.1"}) // VPN DNS
	_, vpnServers, _ := TakeDNSSavedForTest("haovpn0")
	if len(vpnServers) != 1 || vpnServers[0] != "10.88.0.1" {
		t.Fatalf("unexpected %v", vpnServers)
	}
	if DNSSavedCount() != 0 {
		t.Fatal("after restore take, saved must stay empty")
	}
}

// TestSelectProductFilterIDs 仅删除本产品子层过滤器。
func TestSelectProductFilterIDs(t *testing.T) {
	product := HaoVPNKillSublayerBytes()
	other := product
	other[0] ^= 0xff
	got := SelectProductFilterIDs([]WFPFilterRef{
		{ID: 1, Sublayer: other},
		{ID: 2, Sublayer: product},
		{ID: 3, Sublayer: product},
	}, product)
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("got %v", got)
	}
}
