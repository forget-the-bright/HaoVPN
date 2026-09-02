package persist_test

import (
	"path/filepath"
	"strings"
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

// TestAddPeerAccessRequiresVPNUsers 互访双方须存在且为 VPN 账号。
func TestAddPeerAccessRequiresVPNUsers(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer-access-vpn.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := createPeerTestUser(t, store, "acc-a", "10.88.0.40")
	if err := store.AddPeerAccess(a, 99999); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("不存在对端应失败: %v", err)
	}
	hash, _ := auth.HashPassword("Pass12345!")
	adminID, err := store.CreateUser("only-admin", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddPeerAccess(a, adminID); err == nil || !strings.Contains(err.Error(), "VPN") {
		t.Fatalf("纯管理员对端应失败: %v", err)
	}
	b := createPeerTestUser(t, store, "acc-b", "10.88.0.41")
	if err := store.AddPeerAccess(a, b); err != nil {
		t.Fatal(err)
	}
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

// TestUnionMemberUserIDs 任一侧 all 则结果为 all；否则并集去重。
func TestUnionMemberUserIDs(t *testing.T) {
	u := persist.UnionMemberUserIDs([]int64{1, 2}, []int64{2, 3})
	if len(u) != 3 {
		t.Fatalf("want 3, got %v", u)
	}
	u = persist.UnionMemberUserIDs([]int64{persist.PeerRouteMemberAll}, []int64{1})
	if len(u) != 1 || u[0] != persist.PeerRouteMemberAll {
		t.Fatalf("want [0], got %v", u)
	}
}

// TestSymmetricDiffUserIDs 排除名单对称差。
func TestSymmetricDiffUserIDs(t *testing.T) {
	d := persist.SymmetricDiffUserIDs([]int64{1, 2}, []int64{2, 3})
	got := map[int64]bool{}
	for _, id := range d {
		got[id] = true
	}
	if !got[1] || !got[3] || got[2] || len(d) != 2 {
		t.Fatalf("want {1,3} got %v", d)
	}
}

// TestInsertPeerRouteRejectsUnknownMember 不存在的访问方须拒绝。
func TestInsertPeerRouteRejectsUnknownMember(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "mem.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	via := createPeerTestUser(t, store, "viax", "10.88.0.50")
	if _, err := store.InsertPeerRoute("10.1.0.0/24", via, []int64{99999}); err == nil {
		t.Fatal("未知成员应失败")
	}
}

// TestLanRegistryHostIDClamp 超长 host_id 截断入库。
func TestLanRegistryHostIDClamp(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "host.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	u := createPeerTestUser(t, store, "viah", "10.88.0.51")
	long := strings.Repeat("a", persist.MaxLANRegistryHostIDLen+40)
	if err := store.ReplaceClientLANRegistry(u, "10.88.0.51", long, []string{"192.168.9.0/24"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListClientLANRegistry(u)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v err=%v", list, err)
	}
	if len(list[0].HostID) != persist.MaxLANRegistryHostIDLen {
		t.Fatalf("host_id len=%d want %d", len(list[0].HostID), persist.MaxLANRegistryHostIDLen)
	}
}
