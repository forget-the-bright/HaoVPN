package winnet_test

import (
	"net"
	"testing"

	"haovpn/internal/winnet"
)

// TestEgressSnapshotResolveByAddr 同网段 IP 命中优先于路由。
func TestEgressSnapshotResolveByAddr(t *testing.T) {
	snap := &winnet.EgressSnapshot{
		Addrs: []winnet.EgressAddr{
			{IfIndex: 12, Name: "WLAN", IP: net.ParseIP("192.168.31.10").To4(), PrefixLen: 24},
		},
		Routes: []winnet.EgressRoute{
			{IfIndex: 99, Dest: net.IPv4zero, PrefixLen: 0, Metric: 1, IsDefault: true},
		},
		ByIndex: map[int]string{12: "WLAN", 99: "Eth0"},
	}
	name, viaDef, err := snap.ResolveOutboundNatural("192.168.31.0/24")
	if err != nil || name != "WLAN" || viaDef {
		t.Fatalf("got name=%q viaDef=%v err=%v", name, viaDef, err)
	}
}

// TestEgressSnapshotResolveByRoute 无同网段 IP 时走专用路由，再回退默认。
func TestEgressSnapshotResolveByRoute(t *testing.T) {
	snap := &winnet.EgressSnapshot{
		Addrs: []winnet.EgressAddr{
			{IfIndex: 5, Name: "WLAN", IP: net.ParseIP("192.168.31.10").To4(), PrefixLen: 24},
		},
		Routes: []winnet.EgressRoute{
			{IfIndex: 8, Dest: net.ParseIP("192.168.5.0").To4(), PrefixLen: 24, Metric: 10},
			{IfIndex: 5, Dest: net.IPv4zero, PrefixLen: 0, Metric: 25, IsDefault: true},
		},
		ByIndex: map[int]string{5: "WLAN", 8: "ZeroTier"},
	}
	name, viaDef, err := snap.ResolveOutboundNatural("192.168.5.0/24")
	if err != nil || name != "ZeroTier" || viaDef {
		t.Fatalf("dedicated: name=%q viaDef=%v err=%v", name, viaDef, err)
	}
	name, viaDef, err = snap.ResolveOutboundNatural("10.9.9.0/24")
	if err != nil || name != "WLAN" || !viaDef {
		t.Fatalf("default: name=%q viaDef=%v err=%v", name, viaDef, err)
	}
}

// TestEgressSnapshotInterfaceExists 友好名存在性。
func TestEgressSnapshotInterfaceExists(t *testing.T) {
	snap := &winnet.EgressSnapshot{ByIndex: map[int]string{1: "WLAN"}}
	if !snap.InterfaceExistsInSnapshot("wlan") {
		t.Fatal("case-insensitive match expected")
	}
	if snap.InterfaceExistsInSnapshot("Eth0") {
		t.Fatal("missing iface")
	}
}
