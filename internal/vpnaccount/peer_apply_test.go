package vpnaccount_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

func TestPeerPolicyApplierMarkAndApply(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer-apply.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: "pk1", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.10", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var kicked []int64
	ap := vpnaccount.NewPeerPolicyApplier(store, func(uid int64) { kicked = append(kicked, uid) })
	ap.SetListOnline(func() []int64 { return []int64{id} })
	ap.MarkUsers(id)
	pending, _, ids := ap.Status()
	if !pending || len(ids) != 1 {
		t.Fatalf("pending=%v ids=%v", pending, ids)
	}
	res, err := ap.Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kicked != 1 || len(kicked) != 1 || kicked[0] != id {
		t.Fatalf("res=%+v kicked=%v", res, kicked)
	}
	pending, _, _ = ap.Status()
	if pending {
		t.Fatal("应用成功后应无 pending")
	}
	u, _ := store.GetUserByID(id)
	if u.PolicyVer < 1 {
		t.Fatalf("policy_ver 应递增, got %d", u.PolicyVer)
	}
}

func TestPeerPolicyApplierMarkMembersAll(t *testing.T) {
	ap := vpnaccount.NewPeerPolicyApplier(nil, nil)
	ap.MarkMembers([]int64{persist.PeerRouteMemberAll})
	pending, all, _ := ap.Status()
	if !pending || !all {
		t.Fatalf("全部成员应 MarkAll pending=%v all=%v", pending, all)
	}
}

// TestPeerPolicyApplierApplyOnlineOnly MarkAll 时只踢 ListOnline 返回的账号，离线清脏不 Kick。
func TestPeerPolicyApplierApplyOnlineOnly(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer-apply-online.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	mk := func(name, ip, pk string) int64 {
		id, err := store.CreateVPNAccount(persist.User{
			Username: name, PasswordHash: hash, PublicKey: pk, PrivateKeyEnc: "sk",
			VPNIP: ip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := mk("a", "10.88.0.11", "pka")
	b := mk("b", "10.88.0.12", "pkb")
	_ = mk("c", "10.88.0.13", "pkc") // 离线

	var kicked []int64
	ap := vpnaccount.NewPeerPolicyApplier(store, func(uid int64) { kicked = append(kicked, uid) })
	ap.SetListOnline(func() []int64 { return []int64{a, b} })
	ap.MarkAll()
	res, err := ap.Apply(false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kicked != 2 {
		t.Fatalf("应只踢 2 个在线, got kicked=%d res=%+v", res.Kicked, res)
	}
	if len(kicked) != 2 {
		t.Fatalf("Kick 调用次数=%d want 2", len(kicked))
	}
	pending, all, _ := ap.Status()
	if pending || all {
		t.Fatalf("全员在线批处理成功后应清 all, pending=%v all=%v", pending, all)
	}
}

// TestDNSExcludeDirtySymmetricDiff 仅改排除 → 脏标为对称差，非 MarkAll。
func TestDNSExcludeDirtySymmetricDiff(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-ex-dirty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	mk := func(name, ip, pk string) int64 {
		id, err := store.CreateVPNAccount(persist.User{
			Username: name, PasswordHash: hash, PublicKey: pk, PrivateKeyEnc: "sk",
			VPNIP: ip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	u1 := mk("u1", "10.88.0.21", "pk1")
	u2 := mk("u2", "10.88.0.22", "pk2")
	u3 := mk("u3", "10.88.0.23", "pk3")

	ap := vpnaccount.NewPeerPolicyApplier(store, nil)
	d, err := ap.CreateDNSServer(vpnaccount.CreateDNSServerInput{
		DNSIP: "10.10.6.51", ApplyAll: true, ExcludeUserIDs: []int64{u1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ap.Clear() // 忽略创建时的 MarkAll，只测排除差集

	if _, err := ap.ReplaceDNSServerExcludes(d.ID, []int64{u2, u3}); err != nil {
		t.Fatal(err)
	}
	pending, all, ids := ap.Status()
	if !pending || all {
		t.Fatalf("排除变更不得 MarkAll pending=%v all=%v", pending, all)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	// 对称差：去掉 u1，加上 u2/u3
	if !got[u1] || !got[u2] || !got[u3] || len(ids) != 3 {
		t.Fatalf("对称差应为 u1,u2,u3 got=%v", ids)
	}
}

// TestDNSRemarkNoDirty 改备注不标脏。
func TestDNSRemarkNoDirty(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-remark.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	_, err = store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: "pk1", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.30", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ap := vpnaccount.NewPeerPolicyApplier(store, nil)
	d, err := ap.CreateDNSServer(vpnaccount.CreateDNSServerInput{
		DNSIP: "1.1.1.1", ApplyAll: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ap.Clear()
	if _, err := ap.UpdateDNSServerRemark(d.ID, "公司 DNS"); err != nil {
		t.Fatal(err)
	}
	pending, _, _ := ap.Status()
	if pending {
		t.Fatal("改备注不应 pending")
	}
}

// TestDNSMembersDirtyUnion 替换包含集：指定范围 dirty=旧∪新；切到 all 则 MarkAll。
func TestDNSMembersDirtyUnion(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "dns-mem-dirty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	mk := func(name, ip, pk string) int64 {
		id, err := store.CreateVPNAccount(persist.User{
			Username: name, PasswordHash: hash, PublicKey: pk, PrivateKeyEnc: "sk",
			VPNIP: ip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	u1 := mk("m1", "10.88.0.41", "pk1")
	u2 := mk("m2", "10.88.0.42", "pk2")
	u3 := mk("m3", "10.88.0.43", "pk3")

	ap := vpnaccount.NewPeerPolicyApplier(store, nil)
	d, err := ap.CreateDNSServer(vpnaccount.CreateDNSServerInput{
		DNSIP: "9.9.9.9", MemberUserIDs: []int64{u1, u2},
	})
	if err != nil {
		t.Fatal(err)
	}
	ap.Clear()
	if _, err := ap.ReplaceDNSServerMembers(d.ID, []int64{u2, u3}); err != nil {
		t.Fatal(err)
	}
	pending, all, ids := ap.Status()
	if !pending || all {
		t.Fatalf("指定范围应收窄/扩大 dirty，非 all pending=%v all=%v", pending, all)
	}
	got := map[int64]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got[u1] || !got[u2] || !got[u3] {
		t.Fatalf("dirty 须含旧∪新 u1,u2,u3 got=%v", ids)
	}

	ap.Clear()
	if _, err := ap.ReplaceDNSServerMembers(d.ID, []int64{persist.DNSMemberAll}); err != nil {
		t.Fatal(err)
	}
	pending, all, _ = ap.Status()
	if !pending || !all {
		t.Fatalf("切到全部应 MarkAll pending=%v all=%v", pending, all)
	}
}
