package sessionmgr

import (
	"net"
	"testing"

	"haovpn/internal/crypto"
	"haovpn/internal/persist"
)

// TestLateralVPNIPBlocked 禁止账号 A 向账号 B 的 VPN 虚拟 IP 发包。
func TestLateralVPNIPBlocked(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	m := New(store)
	m.vpnIndex = map[string]int64{
		"10.88.0.2": 1,
		"10.88.0.3": 2,
	}

	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)

	_, n, _ := net.ParseCIDR("192.168.1.0/24")
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.2", Crypto: sess, AllowedIPs: []*net.IPNet{n},
	}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	copy(pkt[12:16], net.ParseIP("10.88.0.2").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.3").To4())

	written := false
	err = m.HandleInbound(1, mustEncrypt(t, sess, pkt), func(b []byte) error {
		written = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("lateral vpn ip packet must be dropped")
	}
}

// TestSpoofedSourceIPBlocked 伪造源 IP 应被丢弃。
func TestSpoofedSourceIPBlocked(t *testing.T) {
	store, _ := persist.Open(t.TempDir() + "/spoof.db")
	defer store.Close()
	m := New(store)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	_, n, _ := net.ParseCIDR("192.168.1.0/24")
	m.mu.Lock()
	m.sessions[1] = &AccountSession{UserID: 1, VPNIP: "10.88.0.2", Crypto: sess, AllowedIPs: []*net.IPNet{n}}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	copy(pkt[12:16], net.ParseIP("10.88.0.99").To4()) // 错误源 IP
	copy(pkt[16:20], net.ParseIP("192.168.1.1").To4())

	written := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess, pkt), func(b []byte) error {
		written = true
		return nil
	})
	if written {
		t.Fatal("spoofed source ip must be dropped")
	}
}

// TestDstOutsideAllowedBlocked 目的 IP 不在 AllowedIPs 应丢弃。
func TestDstOutsideAllowedBlocked(t *testing.T) {
	store, _ := persist.Open(t.TempDir() + "/dst.db")
	defer store.Close()
	m := New(store)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	_, n, _ := net.ParseCIDR("192.168.1.0/24")
	m.mu.Lock()
	m.sessions[1] = &AccountSession{UserID: 1, VPNIP: "10.88.0.2", Crypto: sess, AllowedIPs: []*net.IPNet{n}}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	copy(pkt[12:16], net.ParseIP("10.88.0.2").To4())
	copy(pkt[16:20], net.ParseIP("8.8.8.8").To4())

	written := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess, pkt), func(b []byte) error {
		written = true
		return nil
	})
	if written {
		t.Fatal("dst outside allowed_ips must be dropped")
	}
}

// TestExitLANSourceForwardsToPeer via 出口 LAN 源回程应直转到目的 VPN 会话。
func TestExitLANSourceForwardsToPeer(t *testing.T) {
	m := New(nil)
	m.vpnIndex = map[string]int64{"10.88.0.2": 3, "10.88.0.87": 2}
	kp3, _ := crypto.GenerateKeyPair()
	kp2, _ := crypto.GenerateKeyPair()
	sess3, _ := crypto.NewSession(kp3.PrivateKey, kp3.PublicKey)
	sess2, _ := crypto.NewSession(kp2.PrivateKey, kp2.PublicKey)
	_, lan, _ := net.ParseCIDR("192.168.3.0/24")
	_, allowed, _ := net.ParseCIDR("10.0.0.0/8") // 故意不含对端 VPN，验证回程不走 dstAllowed
	conn2 := &captureConn{}
	m.mu.Lock()
	m.sessions[3] = &AccountSession{
		UserID: 3, VPNIP: "10.88.0.2", Crypto: sess3,
		AllowedIPs: []*net.IPNet{allowed}, ExitLANs: []*net.IPNet{lan},
		Conn: &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.87", Crypto: sess2, Conn: conn2,
	}
	m.byIP["10.88.0.87"] = m.sessions[2]
	// 会话 3 须为已应用托管路由的 via，才允许 ExitLAN→对端 VPN 直转
	m.viaIndex = []viaRouteEntry{{net: lan, viaUserID: 3}}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("192.168.3.1").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.87").To4())
	wroteTUN := false
	_ = m.HandleInbound(3, mustEncrypt(t, sess3, pkt), func(b []byte) error {
		wroteTUN = true
		return nil
	})
	if wroteTUN {
		t.Fatal("via 回程不应 writeTUN")
	}
	if conn2.sends == 0 {
		t.Fatal("ExitLANs 源应直转到对端 VPN 会话")
	}
}

// TestExitLANNonViaCannotBypassPeerIsolation 非 via 即使有 ExitLANs 也不得绕过横向隔离直转到对端 VPN IP。
func TestExitLANNonViaCannotBypassPeerIsolation(t *testing.T) {
	m := New(nil)
	m.vpnIndex = map[string]int64{"10.88.0.2": 1, "10.88.0.3": 2}
	kp1, _ := crypto.GenerateKeyPair()
	kp2, _ := crypto.GenerateKeyPair()
	sess1, _ := crypto.NewSession(kp1.PrivateKey, kp1.PublicKey)
	sess2, _ := crypto.NewSession(kp2.PrivateKey, kp2.PublicKey)
	_, lan, _ := net.ParseCIDR("192.168.3.0/24")
	_, allowed, _ := net.ParseCIDR("10.0.0.0/8")
	conn2 := &captureConn{}
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.2", Crypto: sess1,
		AllowedIPs: []*net.IPNet{allowed}, ExitLANs: []*net.IPNet{lan},
		Conn: &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.3", Crypto: sess2, Conn: conn2,
	}
	m.byIP["10.88.0.3"] = m.sessions[2]
	// viaIndex 为空：会话 1 不是任何托管路由的 via
	m.viaIndex = nil
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("192.168.3.1").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.3").To4())
	wroteTUN := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess1, pkt), func(b []byte) error {
		wroteTUN = true
		return nil
	})
	if wroteTUN {
		t.Fatal("非 via ExitLAN 回程不应 writeTUN")
	}
	if conn2.sends != 0 {
		t.Fatal("非 via 不得用 ExitLAN 绕过 peer_access 直转到对端")
	}
}

// TestICSSourceStillBlocked ICS 网段未在 ExitLANs 时仍丢弃。
func TestICSSourceStillBlocked(t *testing.T) {
	m := New(nil)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	_, lan, _ := net.ParseCIDR("192.168.3.0/24")
	_, allowed, _ := net.ParseCIDR("192.168.3.0/24")
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.2", Crypto: sess,
		AllowedIPs: []*net.IPNet{allowed}, ExitLANs: []*net.IPNet{lan},
	}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("192.168.137.1").To4())
	copy(pkt[16:20], net.ParseIP("192.168.3.10").To4())
	written := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess, pkt), func(b []byte) error {
		written = true
		return nil
	})
	if written {
		t.Fatal("ICS 源不在 ExitLANs 必须丢弃")
	}
}

// TestLimitedBroadcastDstDebugOnly 255.255.255.255 应丢弃且不计入 writeTUN。
func TestLimitedBroadcastDstDebugOnly(t *testing.T) {
	m := New(nil)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	_, n, _ := net.ParseCIDR("192.168.1.0/24")
	m.mu.Lock()
	m.sessions[1] = &AccountSession{UserID: 1, VPNIP: "10.88.0.2", Crypto: sess, AllowedIPs: []*net.IPNet{n}}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.2").To4())
	copy(pkt[16:20], net.ParseIP("255.255.255.255").To4())
	written := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess, pkt), func(b []byte) error {
		written = true
		return nil
	})
	if written {
		t.Fatal("受限广播应丢弃")
	}
}

func mustEncrypt(t *testing.T, s *crypto.Session, plain []byte) []byte {
	t.Helper()
	enc, err := s.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}
