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

func mustEncrypt(t *testing.T, s *crypto.Session, plain []byte) []byte {
	t.Helper()
	enc, err := s.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}
