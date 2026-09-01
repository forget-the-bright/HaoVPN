package tunnel

import (
	"strings"
	"testing"

	"haovpn/internal/crypto"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

func TestEncodeParseLANRegistryUpdate(t *testing.T) {
	raw, err := EncodeLANRegistryUpdate([]string{"192.168.31.0/24"}, "host1")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := ParseLANRegistryUpdate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != "lan_registry" || msg.HostID != "host1" || len(msg.LocalLANs) != 1 {
		t.Fatalf("%+v", msg)
	}
	if _, err := ParseLANRegistryUpdate([]byte(`{"type":"handshake"}`)); err == nil {
		t.Fatal("非 lan_registry 应失败")
	}
	if !strings.Contains(string(raw), "lan_registry") {
		t.Fatalf("raw=%s", raw)
	}
}

// TestApplyLANRegistrySyncRateLimited 同会话连续两次 sync，第二次须 rate_limited 且不改注册表。
func TestApplyLANRegistrySyncRateLimited(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/lan_sync.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := sessionmgr.New(store)

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "via", PasswordHash: "x", PublicKey: kp.PublicKey, PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.5", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	user := &persist.User{ID: uid, Username: "via", PublicKey: kp.PublicKey, VPNIP: "10.88.0.5", Enabled: true}
	conn := nopTunnelConn{}
	if err := m.RegisterVPN(user, []string{"10.88.0.0/24"}, conn, sess, "1.1.1.1:1", sessionmgr.PeerReg{}); err != nil {
		t.Fatal(err)
	}
	h := &ServerHandler{Store: store, SessMgr: m, VPNSubnet: "10.88.0.0/24"}
	raw1, _ := EncodeLANRegistryUpdate([]string{"192.168.1.0/24"}, "h")
	h.applyLANRegistrySync(uid, "10.88.0.5", raw1)
	rows, _ := store.ListClientLANRegistry(uid)
	if len(rows) != 1 {
		t.Fatalf("首次 sync 应写入注册表，got %d", len(rows))
	}
	raw2, _ := EncodeLANRegistryUpdate([]string{"192.168.9.0/24"}, "h")
	h.applyLANRegistrySync(uid, "10.88.0.5", raw2)
	rows2, _ := store.ListClientLANRegistry(uid)
	if len(rows2) != 1 || rows2[0].DestCIDR != "192.168.1.0/24" {
		t.Fatalf("rate limit 后注册表应保持首次结果，got %+v", rows2)
	}
}

// nopTunnelConn 满足 sessionmgr.PacketConn，供 tunnel 包单测挂会话。
type nopTunnelConn struct{}

func (nopTunnelConn) Send([]byte) error  { return nil }
func (nopTunnelConn) Close() error       { return nil }
func (nopTunnelConn) RemoteAddr() string { return "1.1.1.1:1" }
