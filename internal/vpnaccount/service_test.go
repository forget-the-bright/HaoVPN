package vpnaccount_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

func testSvc(t *testing.T) (*vpnaccount.Service, *persist.Store, *ippool.Pool) {
	t.Helper()
	store, err := persist.Open(filepath.Join(t.TempDir(), "vpn.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve("10.88.0.1")
	cfg := &config.ServerConfig{
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24"},
		NAT:      config.NATSection{AllowedLANCIDRs: []string{"192.168.31.0/24"}},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	return &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}, store, pool
}

// TestDynamicSessionReleaseOnDisconnect 断线立即回池。
func TestDynamicSessionReleaseOnDisconnect(t *testing.T) {
	svc, store, pool := testSvc(t)
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "dyn", PasswordHash: hash, PublicKey: "pk-dyn", PrivateKeyEnc: "priv",
		IPMode: persist.IPModeDynamicSession, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := store.GetUserByID(id)
	ip, err := svc.EnsureVPNIP(u)
	if err != nil || ip == "" {
		t.Fatalf("allocate: %v %s", err, ip)
	}
	if !pool.IsAllocated(ip) {
		t.Fatal("should be allocated")
	}
	svc.ReleaseOnDisconnect(id, ip, persist.IPModeDynamicSession)
	if pool.IsAllocated(ip) {
		t.Fatal("dynamic_session 断线应释放 IP")
	}
}

// TestDynamicLeaseReuseWithinLease 租约内重连复用同 IP。
func TestDynamicLeaseReuseWithinLease(t *testing.T) {
	svc, store, pool := testSvc(t)
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "lease", PasswordHash: hash, PublicKey: "pk-lease", PrivateKeyEnc: "priv",
		IPMode: persist.IPModeDynamicLease, IPLeaseSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := store.GetUserByID(id)
	ip1, err := svc.EnsureVPNIP(u)
	if err != nil {
		t.Fatal(err)
	}
	svc.ReleaseOnDisconnect(id, ip1, persist.IPModeDynamicLease)
	u2, _ := store.GetUserByID(id)
	ip2, err := svc.EnsureVPNIP(u2)
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != ip2 {
		t.Fatalf("租约内应复用 IP: %s vs %s", ip1, ip2)
	}
	if !pool.IsAllocated(ip2) {
		t.Fatal("复用后应在池中占用")
	}
}

// TestDynamicLeaseNoCrossUserCollision 租约期内其他用户不得占用已租约 IP。
func TestDynamicLeaseNoCrossUserCollision(t *testing.T) {
	svc, store, pool := testSvc(t)
	defer store.Close()
	hash, _ := auth.HashPassword("pass12345")
	id1, err := store.CreateVPNAccount(persist.User{
		Username: "lease1", PasswordHash: hash, PublicKey: "pk-lease1", PrivateKeyEnc: "priv",
		IPMode: persist.IPModeDynamicLease, IPLeaseSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := store.CreateVPNAccount(persist.User{
		Username: "lease2", PasswordHash: hash, PublicKey: "pk-lease2", PrivateKeyEnc: "priv",
		IPMode: persist.IPModeDynamicLease, IPLeaseSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	u1, _ := store.GetUserByID(id1)
	ip1, err := svc.EnsureVPNIP(u1)
	if err != nil {
		t.Fatal(err)
	}
	svc.ReleaseOnDisconnect(id1, ip1, persist.IPModeDynamicLease)
	if !pool.IsAllocated(ip1) {
		t.Fatal("租约断线后 IP 仍应在池中占用")
	}
	u2, _ := store.GetUserByID(id2)
	ip2, err := svc.EnsureVPNIP(u2)
	if err != nil {
		t.Fatal(err)
	}
	if ip2 == ip1 {
		t.Fatalf("其他用户不应获得租约中的 IP: %s", ip1)
	}
}

// TestDefaultAllowedIPs 空账号策略解析为服务端模板。
func TestDefaultAllowedIPs(t *testing.T) {
	svc, store, _ := testSvc(t)
	defer store.Close()
	got := svc.ResolveAllowedIPs(&persist.User{AllowedIPs: nil})
	if len(got) < 2 {
		t.Fatalf("expected lan+subnet, got %v", got)
	}
}
