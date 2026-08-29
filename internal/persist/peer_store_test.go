package persist_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

func createPeerTestUser(t *testing.T, store *persist.Store, name, ip string) int64 {
	t.Helper()
	hash, _ := auth.HashPassword("Pass12345!")
	id, err := store.CreateVPNAccount(persist.User{
		Username: name, PasswordHash: hash, PublicKey: "pk-" + name, PrivateKeyEnc: "sk",
		VPNIP: ip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestPeerAccessAndRoutesCascade 互访与托管路由在删号时须级联清理。
func TestPeerAccessAndRoutesCascade(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	a := createPeerTestUser(t, store, "alice", "10.88.0.10")
	b := createPeerTestUser(t, store, "bob", "10.88.0.20")
	if err := store.AddPeerAccess(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPeerRoute("192.168.0.0/24", b, []int64{a}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPeerRoute("192.168.3.0/24", b, []int64{persist.PeerRouteMemberAll}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteUser(b); err != nil {
		t.Fatal(err)
	}
	acc, _ := store.ListPeerAccessForUser(a)
	if len(acc) != 0 {
		t.Fatalf("peer_access should clear, got %d", len(acc))
	}
	routes, _ := store.ListPeerRoutes()
	if len(routes) != 0 {
		t.Fatalf("peer_routes via bob should clear, got %d", len(routes))
	}
}

// TestListPeerRoutesForAccessor 全部绑定 + 本用户路由合并；指定他人不命中。
func TestListPeerRoutesForAccessor(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := createPeerTestUser(t, store, "a1", "10.88.0.11")
	b := createPeerTestUser(t, store, "b1", "10.88.0.21")
	c := createPeerTestUser(t, store, "c1", "10.88.0.31")
	_, _ = store.InsertPeerRoute("10.0.1.0/24", b, []int64{a})
	_, _ = store.InsertPeerRoute("10.0.2.0/24", b, []int64{persist.PeerRouteMemberAll})
	_, _ = store.InsertPeerRoute("10.0.3.0/24", b, []int64{c})

	got, err := store.ListPeerRoutesForAccessor(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 routes for a, got %d", len(got))
	}
}

// TestPeerRouteMembersAllOverridesSpecific Normalize 后只保留全部。
func TestPeerRouteMembersAllOverridesSpecific(t *testing.T) {
	ids := persist.NormalizeMemberUserIDs([]int64{1, 0, 2, 1})
	if len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("want [0], got %v", ids)
	}
}

// TestLanRegistryReplaceAndClear 换机覆盖与断线清空。
func TestLanRegistryReplaceAndClear(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "lan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	u := createPeerTestUser(t, store, "via1", "10.88.0.2")
	if err := store.ReplaceClientLANRegistry(u, "10.88.0.2", "host-a", []string{"192.168.31.0/24", "10.0.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListClientLANRegistry(u)
	if err != nil || len(list) != 2 {
		t.Fatalf("want 2, got %d err=%v", len(list), err)
	}
	ok, err := store.HasLanRegistryMatch(u, "192.168.31.0/24")
	if err != nil || !ok {
		t.Fatalf("match want true err=%v", err)
	}
	// 换机覆盖：旧网段消失
	if err := store.ReplaceClientLANRegistry(u, "10.88.0.2", "host-b", []string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
	list, _ = store.ListClientLANRegistry(0)
	if len(list) != 1 || list[0].DestCIDR != "192.168.1.0/24" {
		t.Fatalf("overlay failed: %+v", list)
	}
	ok, _ = store.HasLanRegistryMatch(u, "192.168.31.0/24")
	if ok {
		t.Fatal("old cidr should be gone")
	}
	if err := store.ClearClientLANRegistry(u); err != nil {
		t.Fatal(err)
	}
	list, _ = store.ListClientLANRegistry(0)
	if len(list) != 0 {
		t.Fatalf("cleared want 0 got %d", len(list))
	}
}

// TestAddPeerAccessPairBidirectional 双向互访写入与成对删除。
func TestAddPeerAccessPairBidirectional(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "pair.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := createPeerTestUser(t, store, "pa", "10.88.0.41")
	b := createPeerTestUser(t, store, "pb", "10.88.0.42")
	if err := store.AddPeerAccessPair(a, b); err != nil {
		t.Fatal(err)
	}
	all, err := store.ListAllPeerAccess()
	if err != nil || len(all) != 2 {
		t.Fatalf("want 2 rows, got %d err=%v", len(all), err)
	}
	if err := store.RemovePeerAccessPair(a, b); err != nil {
		t.Fatal(err)
	}
	all, _ = store.ListAllPeerAccess()
	if len(all) != 0 {
		t.Fatalf("pair should clear, got %d", len(all))
	}
}

// TestNormalizePeerRouteDest 单 IP 变 /32；禁默认路由。
func TestNormalizePeerRouteDest(t *testing.T) {
	s, err := persist.NormalizePeerRouteDest("10.88.0.5")
	if err != nil || s != "10.88.0.5/32" {
		t.Fatalf("got %q err=%v", s, err)
	}
	if _, err := persist.NormalizePeerRouteDest("0.0.0.0/0"); err == nil {
		t.Fatal("default route should fail")
	}
}
