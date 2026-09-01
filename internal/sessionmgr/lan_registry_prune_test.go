package sessionmgr

import (
	"net"
	"testing"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/persist"
)

// TestPruneViaRoutesAfterRegistryRemovesStaleDest 注册表收缩后成员 ViaRoutes/viaIndex 须剪掉失效 dest。
func TestPruneViaRoutesAfterRegistryRemovesStaleDest(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/prune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	viaID, err := store.CreateVPNAccount(persist.User{
		Username: "via", PasswordHash: "x", PublicKey: "pk-via", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.10", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := store.CreateVPNAccount(persist.User{
		Username: "mem", PasswordHash: "x", PublicKey: "pk-mem", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.20", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceClientLANRegistry(viaID, "10.88.0.10", "h", []string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}

	m := New(store)
	_, keepLAN, _ := net.ParseCIDR("192.168.1.0/24")
	_, dropLAN, _ := net.ParseCIDR("192.168.2.0/24")
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	connM := &captureConn{}
	connV := &captureConn{}

	m.mu.Lock()
	m.sessions[viaID] = &AccountSession{
		UserID: viaID, VPNIP: "10.88.0.10", Crypto: sess, Conn: connV,
		ExitLANs: []*net.IPNet{keepLAN, dropLAN},
	}
	m.sessions[memberID] = &AccountSession{
		UserID: memberID, VPNIP: "10.88.0.20", Crypto: sess, Conn: connM,
		ViaRoutes: []viaRouteEntry{
			{net: keepLAN, viaUserID: viaID},
			{net: dropLAN, viaUserID: viaID},
		},
	}
	m.rebuildViaIndexLocked()
	m.mu.Unlock()

	affected := m.PruneViaRoutesAfterRegistry(viaID)
	if len(affected) != 1 || affected[0] != memberID {
		t.Fatalf("应仅影响成员 %d，got %v", memberID, affected)
	}

	m.mu.RLock()
	routes := m.sessions[memberID].ViaRoutes
	idx := append([]viaRouteEntry(nil), m.viaIndex...)
	m.mu.RUnlock()
	if len(routes) != 1 || routes[0].net.String() != "192.168.1.0/24" {
		t.Fatalf("成员 ViaRoutes 应只剩 keepLAN，got %+v", routes)
	}
	if len(idx) != 1 || idx[0].net.String() != "192.168.1.0/24" {
		t.Fatalf("viaIndex 应只剩 keepLAN，got %+v", idx)
	}

	// 出站命中已剪 dest 不得再转 via
	pkt := make([]byte, 20)
	copy(pkt[16:20], net.ParseIP("192.168.2.8").To4())
	if m.RouteOutbound(pkt) {
		t.Fatal("已剪 dest 不应再经 viaIndex 转发")
	}
	pktKeep := make([]byte, 20)
	copy(pktKeep[16:20], net.ParseIP("192.168.1.8").To4())
	if !m.RouteOutbound(pktKeep) {
		t.Fatal("保留 dest 仍应转发到 via")
	}
}

// TestAllowLANRegistrySyncRateLimit 同会话短间隔第二次 sync 须拒绝。
func TestAllowLANRegistrySyncRateLimit(t *testing.T) {
	m := New(nil)
	m.mu.Lock()
	m.sessions[7] = &AccountSession{UserID: 7, ConnectedAt: time.Now()}
	m.mu.Unlock()

	if !m.AllowLANRegistrySync(7) {
		t.Fatal("首次应允许")
	}
	if m.AllowLANRegistrySync(7) {
		t.Fatal("短间隔第二次应 rate limit")
	}
	m.mu.Lock()
	m.sessions[7].lastLANRegistrySync = time.Now().Add(-lanRegistryMinInterval - time.Second)
	m.mu.Unlock()
	if !m.AllowLANRegistrySync(7) {
		t.Fatal("间隔足够后应再次允许")
	}
}
