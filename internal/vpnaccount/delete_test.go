package vpnaccount_test

import (
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
)

// TestDeleteAccountReleasesFixedIP 删除 fixed 模式账号须同步释放 IP 池占用。
func TestDeleteAccountReleasesFixedIP(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/del.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	pool, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve("10.88.0.1")

	cfg := &config.ServerConfig{}
	cfg.VPN.Subnet = "10.88.0.0/24"
	cfg.VPN.GatewayIP = "10.88.0.1"

	svc := &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
	res, err := svc.ProvisionWebAccount(vpnaccount.ProvisionInput{
		Username:     "u1",
		PasswordHash: "hash",
		IPMode:       persist.IPModeFixed,
		RequestedIP:  "10.88.0.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pool.IsAllocated("10.88.0.2") {
		t.Fatal("ip should be allocated")
	}

	if err := svc.DeleteAccount(res.UserID); err != nil {
		t.Fatal(err)
	}
	if pool.IsAllocated("10.88.0.2") {
		t.Fatal("ip should be released after delete")
	}
	if _, err := store.GetUserByID(res.UserID); err == nil {
		t.Fatal("user should be deleted")
	}
}
