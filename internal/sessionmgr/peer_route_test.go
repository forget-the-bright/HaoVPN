package sessionmgr

import (
	"errors"
	"net"
	"testing"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/persist"
)

// TestLateralAllowAllPeers 全局开关开启时允许互访对方 VPN IP，并直转到对端会话。
func TestLateralAllowAllPeers(t *testing.T) {
	m := New(nil)
	m.SetAllowAllVPNPeers(true)
	m.vpnIndex = map[string]int64{"10.88.0.2": 1, "10.88.0.3": 2}
	kp1, _ := crypto.GenerateKeyPair()
	kp2, _ := crypto.GenerateKeyPair()
	sess1, _ := crypto.NewSession(kp1.PrivateKey, kp1.PublicKey)
	sess2, _ := crypto.NewSession(kp2.PrivateKey, kp2.PublicKey)
	_, n, _ := net.ParseCIDR("10.88.0.3/32")
	conn2 := &captureConn{}
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.2", Crypto: sess1, AllowedIPs: []*net.IPNet{n},
		Conn: &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.3", Crypto: sess2, Conn: conn2,
	}
	m.byIP["10.88.0.3"] = m.sessions[2]
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.2").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.3").To4())
	wroteTUN := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess1, pkt), func(b []byte) error {
		wroteTUN = true
		return nil
	})
	if wroteTUN {
		t.Fatal("横向互访不应 writeTUN")
	}
	if conn2.sends == 0 {
		t.Fatal("allow_all_vpn_peers 时应直转到对端会话")
	}
}

// TestLateralPeerAccessWhitelist peer_access 白名单放行并直转。
func TestLateralPeerAccessWhitelist(t *testing.T) {
	m := New(nil)
	m.vpnIndex = map[string]int64{"10.88.0.2": 1, "10.88.0.3": 2}
	kp1, _ := crypto.GenerateKeyPair()
	kp2, _ := crypto.GenerateKeyPair()
	sess1, _ := crypto.NewSession(kp1.PrivateKey, kp1.PublicKey)
	sess2, _ := crypto.NewSession(kp2.PrivateKey, kp2.PublicKey)
	_, n, _ := net.ParseCIDR("10.88.0.3/32")
	conn2 := &captureConn{}
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.2", Crypto: sess1, AllowedIPs: []*net.IPNet{n},
		PeerAccess: map[int64]struct{}{2: {}},
		Conn:       &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.3", Crypto: sess2, Conn: conn2,
	}
	m.byIP["10.88.0.3"] = m.sessions[2]
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.2").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.3").To4())
	wroteTUN := false
	_ = m.HandleInbound(1, mustEncrypt(t, sess1, pkt), func(b []byte) error {
		wroteTUN = true
		return nil
	})
	if wroteTUN {
		t.Fatal("横向互访不应 writeTUN")
	}
	if conn2.sends == 0 {
		t.Fatal("peer_access 白名单应直转到对端")
	}
}

// TestLateralViaPeersForwards 托管 via 下一跳允许 ping via 的 VPN IP。
func TestLateralViaPeersForwards(t *testing.T) {
	m := New(nil)
	m.vpnIndex = map[string]int64{"10.88.0.87": 2, "10.88.0.2": 3}
	kp2, _ := crypto.GenerateKeyPair()
	kp3, _ := crypto.GenerateKeyPair()
	sess2, _ := crypto.NewSession(kp2.PrivateKey, kp2.PublicKey)
	sess3, _ := crypto.NewSession(kp3.PrivateKey, kp3.PublicKey)
	_, pool, _ := net.ParseCIDR("10.88.0.0/24")
	conn3 := &captureConn{}
	m.mu.Lock()
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.87", Crypto: sess2, AllowedIPs: []*net.IPNet{pool},
		ViaPeers: map[int64]struct{}{3: {}},
		Conn:     &captureConn{},
	}
	m.sessions[3] = &AccountSession{
		UserID: 3, VPNIP: "10.88.0.2", Crypto: sess3, Conn: conn3,
	}
	m.byIP["10.88.0.2"] = m.sessions[3]
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.87").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.2").To4())
	_ = m.HandleInbound(2, mustEncrypt(t, sess2, pkt), func(b []byte) error {
		t.Fatal("不应 writeTUN")
		return nil
	})
	if conn3.sends == 0 {
		t.Fatal("ViaPeers 应直转 via 会话")
	}
}

// TestRegisterVPNViaPeersAllowsLateral RegisterVPN 写入 ViaPeers 后横向可达。
func TestRegisterVPNViaPeersAllowsLateral(t *testing.T) {
	m := New(nil)
	kpA, _ := crypto.GenerateKeyPair()
	kpB, _ := crypto.GenerateKeyPair()
	sessA, _ := crypto.NewSession(kpA.PrivateKey, kpA.PublicKey)
	sessB, _ := crypto.NewSession(kpB.PrivateKey, kpB.PublicKey)
	ua := &persist.User{ID: 2, Username: "company", PublicKey: kpA.PublicKey, VPNIP: "10.88.0.87", Enabled: true}
	ub := &persist.User{ID: 3, Username: "wanghao", PublicKey: kpB.PublicKey, VPNIP: "10.88.0.2", Enabled: true}
	connB := &captureConn{}
	if err := m.RegisterVPN(ub, []string{"10.88.0.0/24"}, connB, sessB, "1.1.1.1:1", PeerReg{}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterVPN(ua, []string{"10.88.0.0/24"}, &captureConn{}, sessA, "2.2.2.2:2", PeerReg{
		ViaUserIDs: []int64{3},
	}); err != nil {
		t.Fatal(err)
	}
	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.87").To4())
	copy(pkt[16:20], net.ParseIP("10.88.0.2").To4())
	_ = m.HandleInbound(2, mustEncrypt(t, sessA, pkt), func(b []byte) error {
		t.Fatal("不应 writeTUN")
		return nil
	})
	if connB.sends == 0 {
		t.Fatal("RegisterVPN ViaUserIDs 应允许横向并直转")
	}
}

// TestRouteOutboundViaManagedRoute 托管路由 dest→via，禁止用 AllowedIPs 错送。
func TestRouteOutboundViaManagedRoute(t *testing.T) {
	m := New(nil)
	kpA, _ := crypto.GenerateKeyPair()
	kpB, _ := crypto.GenerateKeyPair()
	sessA, _ := crypto.NewSession(kpA.PrivateKey, kpA.PublicKey)
	sessB, _ := crypto.NewSession(kpB.PrivateKey, kpB.PublicKey)

	_, lan, _ := net.ParseCIDR("192.168.0.0/24")
	_, nat, _ := net.ParseCIDR("10.0.0.0/24") // NAT 工控网段，不应出站匹配到 A

	connB := &captureConn{}
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.10", Crypto: sessA, AllowedIPs: []*net.IPNet{nat},
		ViaRoutes: []viaRouteEntry{{net: lan, viaUserID: 2}},
		Conn:      &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.14", Crypto: sessB, Conn: connB,
	}
	m.viaIndex = []viaRouteEntry{{net: lan, viaUserID: 2}}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	copy(pkt[12:16], net.ParseIP("10.88.0.10").To4())
	copy(pkt[16:20], net.ParseIP("192.168.0.5").To4())
	if !m.RouteOutbound(pkt) {
		t.Fatal("托管路由应转发到 via 会话")
	}
	if connB.sends == 0 {
		t.Fatal("应向 via 账号发送")
	}

	// NAT 网段不得因会话 AllowedIPs 送回客户端
	pkt2 := make([]byte, 20)
	copy(pkt2[16:20], net.ParseIP("10.0.0.9").To4())
	if m.RouteOutbound(pkt2) {
		t.Fatal("禁止用 AllowedIPs（NAT）出站匹配")
	}
}

// TestInboundManagedRouteForwardsToVia 入站命中托管路由时直转 via，不写 TUN。
func TestInboundManagedRouteForwardsToVia(t *testing.T) {
	m := New(nil)
	kpA, _ := crypto.GenerateKeyPair()
	kpB, _ := crypto.GenerateKeyPair()
	sessA, _ := crypto.NewSession(kpA.PrivateKey, kpA.PublicKey)
	sessB, _ := crypto.NewSession(kpB.PrivateKey, kpB.PublicKey)
	_, lan, _ := net.ParseCIDR("192.168.0.0/24")
	connB := &captureConn{}
	m.mu.Lock()
	m.sessions[1] = &AccountSession{
		UserID: 1, VPNIP: "10.88.0.10", Crypto: sessA, AllowedIPs: []*net.IPNet{lan},
		ViaRoutes: []viaRouteEntry{{net: lan, viaUserID: 2}},
		ViaPeers:  map[int64]struct{}{2: {}},
		Conn:      &captureConn{},
	}
	m.sessions[2] = &AccountSession{
		UserID: 2, VPNIP: "10.88.0.14", Crypto: sessB, Conn: connB,
	}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	pkt[0] = 0x45
	copy(pkt[12:16], net.ParseIP("10.88.0.10").To4())
	copy(pkt[16:20], net.ParseIP("192.168.0.8").To4())
	wroteTUN := false
	_ = m.HandleInbound(1, mustEncrypt(t, sessA, pkt), func(b []byte) error {
		wroteTUN = true
		return nil
	})
	if wroteTUN {
		t.Fatal("托管路由应直转 via，不应写 TUN")
	}
	if connB.sends == 0 {
		t.Fatal("应向 via 发送")
	}
}

// TestReconnectGraceSameIPInheritsBytes 同 IP grace 顶替并继承 Rx/Tx。
func TestReconnectGraceSameIPInheritsBytes(t *testing.T) {
	m := New(nil)
	m.SetReconnectGrace(60 * time.Second)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 9, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.9", Enabled: true}
	c1, c2 := nopConn{}, nopConn{}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, c1, sess, "203.0.113.1:1000", PeerReg{}); err != nil {
		t.Fatal(err)
	}
	ps, ok := m.GetSession(9)
	if !ok {
		t.Fatal("missing session")
	}
	ps.RxBytes.Store(111)
	ps.TxBytes.Store(222)
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, c2, sess, "203.0.113.1:2000", PeerReg{}); err != nil {
		t.Fatalf("同 IP grace 应顶替: %v", err)
	}
	ps2, _ := m.GetSession(9)
	if ps2.RxBytes.Load() != 111 || ps2.TxBytes.Load() != 222 {
		t.Fatalf("应继承流量 rx=%d tx=%d", ps2.RxBytes.Load(), ps2.TxBytes.Load())
	}
}

// TestReconnectGraceRejectDifferentIP 异 IP 且对端仍活跃时在 reject_second 下仍拒绝。
func TestReconnectGraceRejectDifferentIP(t *testing.T) {
	m := New(nil)
	m.SetReconnectGrace(60 * time.Second)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 10, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.10", Enabled: true}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, &freshActivityConn{}, sess, "203.0.113.1:1", PeerReg{}); err != nil {
		t.Fatal(err)
	}
	err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, nopConn{}, sess, "198.51.100.2:1", PeerReg{})
	if !errors.Is(err, ErrAccountAlreadyOnline) {
		t.Fatalf("异 IP 且活跃应拒绝, got %v", err)
	}
}

// TestReconnectStalePeerAllowsDifferentIP 对端静默超时后允许异 IP 密码重连顶替。
func TestReconnectStalePeerAllowsDifferentIP(t *testing.T) {
	m := New(nil)
	m.SetReconnectGrace(60 * time.Second)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 11, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.11", Enabled: true}
	stale := &staleActivityConn{at: time.Now().Add(-30 * time.Second)}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, stale, sess, "203.0.113.1:1", PeerReg{}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, nopConn{}, sess, "198.51.100.2:1", PeerReg{}); err != nil {
		t.Fatalf("半死会话应允许顶替: %v", err)
	}
}

// TestSameRemoteHostNormalizesLoopback ::1 与 127.0.0.1 视为同主机。
func TestSameRemoteHostNormalizesLoopback(t *testing.T) {
	if !sameRemoteHost("[::1]:1", "127.0.0.1:2") {
		t.Fatal("loopback should match")
	}
}

// TestViaIndexRebuildDeterministic 重叠 dest 时 viaIndex 重建顺序稳定（低 viaUserID 优先）。
func TestViaIndexRebuildDeterministic(t *testing.T) {
	m := New(nil)
	kpA, _ := crypto.GenerateKeyPair()
	kpB, _ := crypto.GenerateKeyPair()
	kpC, _ := crypto.GenerateKeyPair()
	sessA, _ := crypto.NewSession(kpA.PrivateKey, kpA.PublicKey)
	sessB, _ := crypto.NewSession(kpB.PrivateKey, kpB.PublicKey)
	sessC, _ := crypto.NewSession(kpC.PrivateKey, kpC.PublicKey)
	_, lan, _ := net.ParseCIDR("192.168.50.0/24")
	connLow := &captureConn{}
	connHigh := &captureConn{}

	m.mu.Lock()
	m.sessions[10] = &AccountSession{
		UserID: 10, VPNIP: "10.88.0.10", Crypto: sessA,
		ViaRoutes: []viaRouteEntry{{net: lan, viaUserID: 20}},
		Conn:      &captureConn{},
	}
	m.sessions[11] = &AccountSession{
		UserID: 11, VPNIP: "10.88.0.11", Crypto: sessB,
		ViaRoutes: []viaRouteEntry{{net: lan, viaUserID: 30}},
		Conn:      &captureConn{},
	}
	m.sessions[20] = &AccountSession{UserID: 20, VPNIP: "10.88.0.20", Crypto: sessC, Conn: connLow}
	m.sessions[30] = &AccountSession{
		UserID: 30, VPNIP: "10.88.0.30",
		Crypto: func() *crypto.Session { s, _ := crypto.NewSession(kpC.PrivateKey, kpC.PublicKey); return s }(),
		Conn:   connHigh,
	}
	for i := 0; i < 50; i++ {
		m.rebuildViaIndexLocked()
	}
	m.mu.Unlock()

	pkt := make([]byte, 20)
	copy(pkt[16:20], net.ParseIP("192.168.50.9").To4())
	if !m.RouteOutbound(pkt) {
		t.Fatal("重叠托管路由应命中 via")
	}
	if connLow.sends == 0 {
		t.Fatal("稳定排序后应优先低 viaUserID=20")
	}
	if connHigh.sends != 0 {
		t.Fatalf("不应命中高 viaUserID=30, sends=%d", connHigh.sends)
	}
}

type freshActivityConn struct{ nopConn }

func (c *freshActivityConn) LastPeerActivity() time.Time { return time.Now() }

type staleActivityConn struct {
	nopConn
	at time.Time
}

func (c *staleActivityConn) LastPeerActivity() time.Time { return c.at }

type captureConn struct {
	sends int
}

func (c *captureConn) Send([]byte) error  { c.sends++; return nil }
func (c *captureConn) Close() error       { return nil }
func (c *captureConn) RemoteAddr() string { return "1.1.1.1:1" }

type nopConn struct{}

func (nopConn) Send([]byte) error     { return nil }
func (nopConn) Close() error          { return nil }
func (nopConn) RemoteAddr() string    { return "1.2.3.4:1" }
