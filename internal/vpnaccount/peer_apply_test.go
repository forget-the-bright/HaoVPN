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
