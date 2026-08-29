package tunnel_test

import (
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
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

// TestHandshakeRejectSecond 默认 reject_second：第二端须收到「已在其他设备在线」，旧会话保持。
func TestHandshakeRejectSecond(t *testing.T) {
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

	sessMgr := sessionmgr.New(store) // 默认 reject_second
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
				_ = raw.Close()
				continue
			}
			conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
			handler.Attach(conn)
		}
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	cfgT := transport.DefaultConfig()

	c1, err := transport.Dial(ln.Addr().String(), clientTLS, cfgT, nil, nil)
	if err != nil {
		t.Fatalf("dial1: %v", err)
	}
	defer c1.Close()
	if _, err := tunnel.NewClientHandshake().RunAuthWithTimeout(c1, "u1", "testpass12", 10*time.Second); err != nil {
		t.Fatalf("handshake1: %v", err)
	}
	if sessMgr.OnlineCount() != 1 {
		t.Fatalf("online after first=%d", sessMgr.OnlineCount())
	}

	c2, err := transport.Dial(ln.Addr().String(), clientTLS, cfgT, nil, nil)
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	defer c2.Close()
	_, err = tunnel.NewClientHandshake().RunAuthWithTimeout(c2, "u1", "testpass12", 10*time.Second)
	if err == nil || !strings.Contains(err.Error(), "已在其他设备在线") {
		t.Fatalf("第二端应拒绝已在线, err=%v", err)
	}
	if sessMgr.OnlineCount() != 1 {
		t.Fatalf("旧会话应保持 online=%d", sessMgr.OnlineCount())
	}
}

// TestHandshakeKickPreviousNoDeadlock kick_previous 下新连接替换旧连接不得死锁。
func TestHandshakeKickPreviousNoDeadlock(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "kick.db"))
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
		VPNIP: "10.88.0.52", AllowedIPs: []string{"192.168.1.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
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
	sessMgr.SetSessionPolicy(config.SessionPolicyKickPrevious)
	pool, _ := ippool.New("10.88.0.0/24")
	pool.Reserve("10.88.0.1")
	_ = pool.AllocateSpecific("10.88.0.52", uid)
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
				_ = raw.Close()
				continue
			}
			conn := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
			handler.Attach(conn)
		}
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	cfgT := transport.DefaultConfig()

	c1, err := transport.Dial(ln.Addr().String(), clientTLS, cfgT, nil, nil)
	if err != nil {
		t.Fatalf("dial1: %v", err)
	}
	if _, err := tunnel.NewClientHandshake().RunAuthWithTimeout(c1, "u1", "testpass12", 10*time.Second); err != nil {
		t.Fatalf("handshake1: %v", err)
	}

	c2, err := transport.Dial(ln.Addr().String(), clientTLS, cfgT, nil, nil)
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	defer c2.Close()
	if _, err := tunnel.NewClientHandshake().RunAuthWithTimeout(c2, "u1", "testpass12", 10*time.Second); err != nil {
		t.Fatalf("handshake2 (kick): %v", err)
	}
	if sessMgr.OnlineCount() != 1 {
		t.Fatalf("online=%d want 1", sessMgr.OnlineCount())
	}
	_ = c1.Close()
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
