package vpnaccount_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

func testProvisionService(t *testing.T) (*vpnaccount.Service, *persist.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ServerConfig{
		VPN: config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1"},
	}
	pool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve(cfg.VPN.GatewayIP)
	return &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}, store
}

func TestValidateAllowedIPsRejectsFullTunnel(t *testing.T) {
	if err := vpnaccount.ValidateAllowedIPs([]string{"0.0.0.0/0"}); err == nil {
		t.Fatal("expected error for full tunnel")
	}
	if err := vpnaccount.ValidateAllowedIPs([]string{"192.168.1.0/24"}); err != nil {
		t.Fatalf("split tunnel ok: %v", err)
	}
}

func TestValidateManualIP(t *testing.T) {
	svc, store := testProvisionService(t)
	defer store.Close()

	if err := svc.ValidateManualIP("10.88.0.1", 0); err == nil {
		t.Fatal("gateway must be rejected")
	}
	if err := svc.ValidateManualIP("10.88.0.50", 0); err != nil {
		t.Fatalf("free ip: %v", err)
	}
	if err := svc.ValidateManualIP("192.168.0.5", 0); err == nil {
		t.Fatal("out of subnet must fail")
	}
}

func TestProvisionWebAccountManualIP(t *testing.T) {
	svc, store := testProvisionService(t)
	defer store.Close()

	res, err := svc.ProvisionWebAccount(vpnaccount.ProvisionInput{
		Username:     "eng1",
		PasswordHash: "hash",
		IPMode:       persist.IPModeFixed,
		RequestedIP:  "10.88.0.55",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.VPNIP != "10.88.0.55" {
		t.Fatalf("vpn_ip=%s", res.VPNIP)
	}
	u, err := store.GetUserByID(res.UserID)
	if err != nil || u.VPNIP != "10.88.0.55" {
		t.Fatalf("user record: %v ip=%s", err, u.VPNIP)
	}
	if !svc.Pool.IsAllocated("10.88.0.55") {
		t.Fatal("pool should mark ip allocated")
	}
}

func TestProvisionWebAccountDynamicRejectsManualIP(t *testing.T) {
	svc, store := testProvisionService(t)
	defer store.Close()

	_, err := svc.ProvisionWebAccount(vpnaccount.ProvisionInput{
		Username:     "dyn",
		PasswordHash: "hash",
		IPMode:       persist.IPModeDynamicSession,
		RequestedIP:  "10.88.0.60",
	})
	if err == nil {
		t.Fatal("dynamic mode must reject manual ip")
	}
}

func TestRebindFixedVPNIP(t *testing.T) {
	svc, store := testProvisionService(t)
	defer store.Close()

	res, err := svc.ProvisionWebAccount(vpnaccount.ProvisionInput{
		Username:     "rb",
		PasswordHash: "hash",
		IPMode:       persist.IPModeFixed,
		RequestedIP:  "10.88.0.61",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RebindFixedVPNIP(res.UserID, "10.88.0.61", "10.88.0.62"); err != nil {
		t.Fatal(err)
	}
	if svc.Pool.IsAllocated("10.88.0.61") {
		t.Fatal("old ip should be released")
	}
	if !svc.Pool.IsAllocated("10.88.0.62") {
		t.Fatal("new ip should be allocated")
	}
}
