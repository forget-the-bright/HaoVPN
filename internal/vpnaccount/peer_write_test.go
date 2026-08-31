package vpnaccount_test

import (
	"errors"
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// TestPeerWriteCreateDelete 创建/删除托管路由经 PeerPolicyApplier 写库并标脏。
func TestPeerWriteCreateDelete(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer_write.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	viaID, err := store.CreateVPNAccount(persist.User{
		Username: "via1", PasswordHash: hash, PublicKey: "pk-via", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.2", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	memID, err := store.CreateVPNAccount(persist.User{
		Username: "mem1", PasswordHash: hash, PublicKey: "pk-mem", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.3", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	applier := vpnaccount.NewPeerPolicyApplier(store, nil)
	res, err := applier.CreatePeerRoute(vpnaccount.CreatePeerRouteInput{
		DestCIDR: "192.168.1.0/24", ViaUserID: viaID, MemberUserIDs: []int64{memID},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, _, ids := applier.Status()
	if !pending || len(ids) == 0 {
		t.Fatalf("expected dirty after create pending=%v ids=%v", pending, ids)
	}
	old, err := applier.DeletePeerRoute(res.ID)
	if err != nil || old == nil {
		t.Fatalf("delete: %v old=%v", err, old)
	}
	if _, err := applier.DeletePeerRoute(res.ID); !errors.Is(err, vpnaccount.ErrPeerRouteNotFound) {
		t.Fatalf("want ErrPeerRouteNotFound got %v", err)
	}
}

// TestPeerWriteViaMustBeVPN via 非 VPN 须拒绝。
func TestPeerWriteViaMustBeVPN(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "peer_via.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, _ := auth.HashPassword("Pass12345!")
	webOnly, err := store.CreateUser("webonly", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	applier := vpnaccount.NewPeerPolicyApplier(store, nil)
	_, err = applier.CreatePeerRoute(vpnaccount.CreatePeerRouteInput{
		DestCIDR: "10.0.0.0/24", ViaUserID: webOnly, MemberUserIDs: []int64{persist.PeerRouteMemberAll},
	})
	if !errors.Is(err, vpnaccount.ErrViaNotVPN) {
		t.Fatalf("want ErrViaNotVPN got %v", err)
	}
}
