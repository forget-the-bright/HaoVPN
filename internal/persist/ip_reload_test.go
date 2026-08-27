package persist_test

import (
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/auth"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
)

// TestIPPoolReloadFromAccounts 模拟服务端重启：从 users 表恢复 IP 池后不得重复分配。
func TestIPPoolReloadFromAccounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reload.db")
	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	hash, _ := auth.HashPassword("testpass12")
	id, err := store.CreateVPNAccount(persist.User{
		Username: "u1", PasswordHash: hash, PublicKey: "pub1", PrivateKeyEnc: "priv1",
		VPNIP: "10.88.0.5", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIPAllocation("10.88.0.5", id); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store2, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	pool, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve("10.88.0.1")
	accounts, err := store2.ListVPNAccounts()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range accounts {
		if u.VPNIP == "" {
			continue
		}
		if err := pool.AllocateSpecific(u.VPNIP, u.ID); err != nil {
			t.Fatalf("reload allocate: %v", err)
		}
	}
	if !pool.IsAllocated("10.88.0.5") {
		t.Fatal("10.88.0.5 should remain allocated after reload")
	}
	ip, err := pool.Allocate(99)
	if err != nil {
		t.Fatal(err)
	}
	if ip == "10.88.0.5" {
		t.Fatal("must not re-allocate existing account IP")
	}
}

// TestIPPoolReloadLeaseOccupancy 重启后 dynamic_lease 占用须入池，他人不得撞车。
func TestIPPoolReloadLeaseOccupancy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "lease-reload.db")
	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("testpass12")
	id1, err := store.CreateVPNAccount(persist.User{
		Username: "lease1", PasswordHash: hash, PublicKey: "pk1", PrivateKeyEnc: "sk1",
		VPNIP: "10.88.0.10", IPMode: persist.IPModeDynamicLease, IPLeaseSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordIPAllocation("10.88.0.10", id1); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Hour)
	if err := store.SetIPLeaseUntil("10.88.0.10", until); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store2, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	pool, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve("10.88.0.1")
	active, err := store2.ListActiveUserIPs()
	if err != nil {
		t.Fatal(err)
	}
	for ip, uid := range active {
		if err := pool.AllocateSpecific(ip, uid); err != nil {
			t.Fatalf("restore %s: %v", ip, err)
		}
	}
	id2, err := store2.CreateVPNAccount(persist.User{
		Username: "lease2", PasswordHash: hash, PublicKey: "pk2", PrivateKeyEnc: "sk2",
		IPMode: persist.IPModeDynamicLease, IPLeaseSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ip2, err := pool.Allocate(id2)
	if err != nil {
		t.Fatal(err)
	}
	if ip2 == "10.88.0.10" {
		t.Fatal("第二用户不得占用租约中的 IP")
	}
}
