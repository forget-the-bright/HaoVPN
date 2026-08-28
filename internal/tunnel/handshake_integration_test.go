package tunnel_test

import (
	"crypto/tls"
	"net"
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/ippool"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
	"haovpn/internal/vpnaccount"
)

func init() {
	_ = logger.Init(logger.Config{Level: "error"})
}

// TestHandshakeIntegration 端到端隧道握手（无 TUN，step11 子项）。
func TestHandshakeIntegration(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "hs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKP, err := tunnel.LoadOrCreateServerKeys(dir)
	if err != nil {
		t.Fatal(err)
	}

	hash, _ := auth.HashPassword("testpass12")
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.50", AllowedIPs: []string{"192.168.1.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = uid

	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sessMgr := sessionmgr.New(store)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	_ = pool.AllocateSpecific("10.88.0.50", 1)
	cfg := &config.ServerConfig{
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", MTU: 1420},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	vpnSvc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	authSvc := auth.New(store, 5, 900, 3600)
	handler := &tunnel.ServerHandler{
		Store: store, SessMgr: sessMgr, ServerKP: serverKP, TunDev: nil, VPN: vpnSvc, MTU: 1420,
		GatewayIP: "10.88.0.1", Auth: authSvc,
	}

	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tc := tls.Server(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			return
		}
		conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
		handler.Attach(conn)
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, transport.DefaultConfig(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs := tunnel.NewClientHandshake()
	res, err := hs.RunAuthWithTimeout(conn, "u1", "testpass12", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ServerPublicKey != serverKP.PublicKey {
		t.Fatalf("server pk mismatch")
	}
	if res.Policy.VPNIP != "10.88.0.50" {
		t.Fatalf("policy vpn_ip=%s", res.Policy.VPNIP)
	}
	if res.Policy.GatewayIP != "10.88.0.1" {
		t.Fatalf("policy gateway_ip=%s", res.Policy.GatewayIP)
	}
	if len(res.Policy.DNSServers) == 0 || res.Policy.DNSServers[0] != "10.88.0.1" {
		t.Fatalf("policy dns_servers=%v", res.Policy.DNSServers)
	}
	if sessMgr.OnlineCount() != 1 {
		t.Fatalf("expected 1 online account")
	}
}

// TestHandshakeDisabledAccountRejected 禁用账号后新握手须失败（meta-plan #6）。
func TestHandshakeDisabledAccountRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "disabled.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKP, err := tunnel.LoadOrCreateServerKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.55", AllowedIPs: []string{"192.168.1.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUserEnabled(uid, false); err != nil {
		t.Fatal(err)
	}

	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sessMgr := sessionmgr.New(store)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	cfg := &config.ServerConfig{
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", MTU: 1420},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	vpnSvc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	authSvc := auth.New(store, 5, 900, 3600)
	handler := &tunnel.ServerHandler{
		Store: store, SessMgr: sessMgr, ServerKP: serverKP, TunDev: nil, VPN: vpnSvc, MTU: 1420,
		GatewayIP: "10.88.0.1", Auth: authSvc,
	}

	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tc := tls.Server(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			return
		}
		conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
		handler.Attach(conn)
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, transport.DefaultConfig(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs := tunnel.NewClientHandshake()
	_, err = hs.RunAuthWithTimeout(conn, "u1", "testpass12", 5*time.Second)
	if err == nil {
		t.Fatal("disabled account should reject handshake")
	}
}

// TestHandshakeReconnectNoDeadlock 同账号新连接替换旧连接时不得死锁。
func TestHandshakeReconnectNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "reconn.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKP, err := tunnel.LoadOrCreateServerKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.51", AllowedIPs: []string{"192.168.1.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sessMgr := sessionmgr.New(store)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	_ = pool.AllocateSpecific("10.88.0.51", uid)
	cfg := &config.ServerConfig{
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", MTU: 1420},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	vpnSvc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	authSvc := auth.New(store, 5, 900, 3600)
	handler := &tunnel.ServerHandler{Store: store, SessMgr: sessMgr, ServerKP: serverKP, VPN: vpnSvc, MTU: 1420, Auth: authSvc}

	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			tc := tls.Server(raw, tlsCfg)
			if err := tc.Handshake(); err != nil {
				continue
			}
			conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
			handler.Attach(conn)
		}
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	cfgT := transport.DefaultConfig()

	dialHandshake := func() *transport.Conn {
		conn, err := transport.Dial(ln.Addr().String(), clientTLS, cfgT, nil, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		hs := tunnel.NewClientHandshake()
		if _, err := hs.RunAuthWithTimeout(conn, "u1", "testpass12", 5*time.Second); err != nil {
			t.Fatalf("handshake: %v", err)
		}
		return conn
	}

	done := make(chan struct{})
	var onlineAtEnd int
	go func() {
		defer close(done)
		c1 := dialHandshake()
		time.Sleep(100 * time.Millisecond)
		c2 := dialHandshake()
		onlineAtEnd = sessMgr.OnlineCount()
		c2.Close()
		c1.Close()
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("同账号重连死锁或超时")
	}
	if onlineAtEnd != 1 {
		t.Fatalf("online=%d want 1", onlineAtEnd)
	}
	st, err := store.GetSessionStat(uid)
	if err != nil {
		t.Fatal(err)
	}
	if st.ReconnectCount < 1 {
		t.Fatalf("reconnect_count=%d want >=1", st.ReconnectCount)
	}
}

// TestHandshakePasswordAuth 账号密码握手须下发 gateway 与 client_private_key。
func TestHandshakePasswordAuth(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "pwd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKP, err := tunnel.LoadOrCreateServerKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	keyEnc, err := security.NewKeyEnc(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := keyEnc.SealPrivateKey(kp.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "eng", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: sealed,
		VPNIP: "10.88.0.60", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sessMgr := sessionmgr.New(store)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	_ = pool.AllocateSpecific("10.88.0.60", uid)
	cfg := &config.ServerConfig{
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", MTU: 1420, GatewayIP: "10.88.0.1"},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	vpnSvc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	authSvc := auth.New(store, 5, 900, 3600)
	handler := &tunnel.ServerHandler{
		Store: store, SessMgr: sessMgr, ServerKP: serverKP, VPN: vpnSvc, MTU: 1420,
		GatewayIP: "10.88.0.1", Auth: authSvc, KeyEnc: keyEnc,
	}

	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tc := tls.Server(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			return
		}
		conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
		handler.Attach(conn)
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, transport.DefaultConfig(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs := tunnel.NewClientHandshake()
	res, err := hs.RunAuthWithTimeout(conn, "eng", "testpass12", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClientPrivateKey != kp.PrivateKey {
		t.Fatalf("client private key not returned")
	}
	if res.Policy.GatewayIP != "10.88.0.1" || res.Policy.VPNIP != "10.88.0.60" {
		t.Fatalf("policy=%+v", res.Policy)
	}
}

// TestHandshakePublicKeyRejected 仅公钥握手须被拒绝。
func TestHandshakePublicKeyRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "pubkey.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	serverKP, err := tunnel.LoadOrCreateServerKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	_, err = store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.70", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	sessMgr := sessionmgr.New(store)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	cfg := &config.ServerConfig{VPN: config.VPNSection{Subnet: "10.88.0.0/24"}}
	vpnSvc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	authSvc := auth.New(store, 5, 900, 3600)
	handler := &tunnel.ServerHandler{
		Store: store, SessMgr: sessMgr, ServerKP: serverKP, VPN: vpnSvc, Auth: authSvc,
	}

	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tc := tls.Server(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			return
		}
		conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
		handler.Attach(conn)
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, transport.DefaultConfig(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs := tunnel.NewClientHandshake()
	_, err = hs.RunWithTimeout(conn, kp.PublicKey, 3*time.Second)
	if err == nil {
		t.Fatal("expected pubkey handshake to fail")
	}
	if sessMgr.OnlineCount() != 0 {
		t.Fatalf("online=%d want 0", sessMgr.OnlineCount())
	}
}

func genTestCert(t *testing.T) tls.Certificate {
	dir := t.TempDir()
	cf := filepath.Join(dir, "c.crt")
	kf := filepath.Join(dir, "c.key")
	if err := security.EnsureServerCert(cf, kf, true, nil); err != nil {
		t.Fatal(err)
	}
	c, err := tls.LoadX509KeyPair(cf, kf)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
