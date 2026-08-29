package vpnaccount_test

import (
	"errors"
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// TestLastAdminCannotDisableOrDelete 最后一个启用管理员不可禁用/删除。
func TestLastAdminCannotDisableOrDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "lastadmin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	if err := authSvc.EnsureAdmin("admin", "AdminPass12!", false); err != nil {
		t.Fatal(err)
	}
	admin, err := store.GetUserByUsername("admin")
	if err != nil || admin == nil {
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
	svc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}

	if err := svc.SetAccountEnabled(admin.ID, false); !errors.Is(err, vpnaccount.ErrLastAdmin) {
		t.Fatalf("disable last admin: got %v", err)
	}
	if err := svc.DeleteAccount(admin.ID); !errors.Is(err, vpnaccount.ErrLastAdmin) {
		t.Fatalf("delete last admin: got %v", err)
	}

	hash, err := auth.HashPassword("OtherPass12!")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := store.CreateAdminUser("admin2", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAccountEnabled(admin.ID, false); err != nil {
		t.Fatalf("disable with second admin: %v", err)
	}
	if err := svc.DeleteAccount(id2); !errors.Is(err, vpnaccount.ErrLastAdmin) {
		t.Fatalf("delete sole remaining admin: got %v", err)
	}
}
