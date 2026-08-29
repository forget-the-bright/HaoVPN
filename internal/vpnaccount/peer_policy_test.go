package vpnaccount_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// TestResolveClientPolicyMergesPeerAndRoutes 合并托管 dest；子网覆盖时不下发冗余 /32。
func TestResolveClientPolicyMergesPeerAndRoutes(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "pol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, _ := auth.HashPassword("Pass12345!")
	// AllowedIPs 为空 → 使用默认 NAT + split 子网 10.88.0.0/24
	aid, err := store.CreateVPNAccount(persist.User{
		Username: "a", PasswordHash: hash, PublicKey: "pka", PrivateKeyEnc: "ska",
		VPNIP: "10.88.0.10", AllowedIPs: nil, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	bid, err := store.CreateVPNAccount(persist.User{
		Username: "b", PasswordHash: hash, PublicKey: "pkb", PrivateKeyEnc: "skb",
		VPNIP: "10.88.0.14", AllowedIPs: nil, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddPeerAccess(aid, bid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPeerRoute("192.168.0.0/24", bid, []int64{aid}); err != nil {
		t.Fatal(err)
	}
	// via 须上报注册表，否则策略视托管路由为失效
	if err := store.ReplaceClientLANRegistry(bid, "10.88.0.14", "t", []string{"192.168.0.0/24"}); err != nil {
		t.Fatal(err)
	}

	svc := &vpnaccount.Service{
		Store: store,
		Cfg: &config.ServerConfig{
			NAT:      config.NATSection{AllowedLANCIDRs: []string{"10.0.0.0/24"}},
			Security: config.SecuritySection{EnforceSplitTunnel: true},
			VPN:      config.VPNSection{Subnet: "10.88.0.0/24"},
		},
	}
	ua, _ := store.GetUserByID(aid)
	pol, err := svc.ResolveClientPolicy(ua)
	if err != nil {
		t.Fatal(err)
	}
	has := func(c string) bool {
		for _, x := range pol.AllowedIPs {
			if x == c {
				return true
			}
		}
		return false
	}
	if !has("10.0.0.0/24") {
		t.Fatalf("应含默认 NAT: %v", pol.AllowedIPs)
	}
	if !has("10.88.0.0/24") {
		t.Fatalf("split 应含 VPN 子网: %v", pol.AllowedIPs)
	}
	if has("10.88.0.14/32") {
		t.Fatalf("已被 /24 覆盖时不应下发冗余 peer/via /32: %v", pol.AllowedIPs)
	}
	if !has("192.168.0.0/24") {
		t.Fatalf("应合并托管 dest: %v", pol.AllowedIPs)
	}
	if len(pol.PeerAccessIDs) != 1 || pol.PeerAccessIDs[0] != bid {
		t.Fatalf("PeerAccessIDs=%v", pol.PeerAccessIDs)
	}
	if len(pol.ViaUserIDs) != 1 || pol.ViaUserIDs[0] != bid {
		t.Fatalf("ViaUserIDs=%v", pol.ViaUserIDs)
	}
	if len(pol.ManagedRoutes) != 1 || pol.ManagedRoutes[0].ViaIP != "10.88.0.14" || pol.ManagedRoutes[0].Stale {
		t.Fatalf("managed_routes=%+v", pol.ManagedRoutes)
	}
}

// TestResolveClientPolicySkipsStaleRoute 无注册表时不下发 dest。
func TestResolveClientPolicySkipsStaleRoute(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "pol-stale.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	aid, _ := store.CreateVPNAccount(persist.User{
		Username: "a", PasswordHash: hash, PublicKey: "pka", PrivateKeyEnc: "ska",
		VPNIP: "10.88.0.10", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	bid, _ := store.CreateVPNAccount(persist.User{
		Username: "b", PasswordHash: hash, PublicKey: "pkb", PrivateKeyEnc: "skb",
		VPNIP: "10.88.0.14", AllowedIPs: nil, IPMode: persist.IPModeFixed, Enabled: true,
	})
	_, _ = store.InsertPeerRoute("192.168.9.0/24", bid, []int64{persist.PeerRouteMemberAll})
	svc := &vpnaccount.Service{
		Store: store,
		Cfg: &config.ServerConfig{
			NAT:      config.NATSection{AllowedLANCIDRs: []string{"10.0.0.0/24"}},
			Security: config.SecuritySection{EnforceSplitTunnel: true},
			VPN:      config.VPNSection{Subnet: "10.88.0.0/24"},
		},
	}
	ua, _ := store.GetUserByID(aid)
	pol, err := svc.ResolveClientPolicy(ua)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range pol.AllowedIPs {
		if c == "192.168.9.0/24" {
			t.Fatalf("stale dest must not be in AllowedIPs: %v", pol.AllowedIPs)
		}
	}
	if len(pol.ManagedRoutes) != 1 || !pol.ManagedRoutes[0].Stale {
		t.Fatalf("want stale managed route, got %+v", pol.ManagedRoutes)
	}
}

// TestResolveClientPolicyPeerHostWhenNoSubnet 无 VPN 子网时须下发 peer /32 以便进隧道。
func TestResolveClientPolicyPeerHostWhenNoSubnet(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "pol2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	aid, err := store.CreateVPNAccount(persist.User{
		Username: "a", PasswordHash: hash, PublicKey: "pka", PrivateKeyEnc: "ska",
		VPNIP: "10.88.0.10", AllowedIPs: []string{"192.168.1.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	bid, err := store.CreateVPNAccount(persist.User{
		Username: "b", PasswordHash: hash, PublicKey: "pkb", PrivateKeyEnc: "skb",
		VPNIP: "10.88.0.14", AllowedIPs: nil, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddPeerAccess(aid, bid); err != nil {
		t.Fatal(err)
	}
	svc := &vpnaccount.Service{
		Store: store,
		Cfg: &config.ServerConfig{
			NAT:      config.NATSection{AllowedLANCIDRs: []string{"192.168.1.0/24"}},
			Security: config.SecuritySection{EnforceSplitTunnel: false},
			VPN:      config.VPNSection{Subnet: "10.88.0.0/24"},
		},
	}
	ua, _ := store.GetUserByID(aid)
	pol, err := svc.ResolveClientPolicy(ua)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range pol.AllowedIPs {
		if c == "10.88.0.14/32" {
			found = true
		}
	}
	if !found {
		t.Fatalf("无子网覆盖时应下发 peer /32: %v", pol.AllowedIPs)
	}
}
