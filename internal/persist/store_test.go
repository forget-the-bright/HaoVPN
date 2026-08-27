package persist_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestStoreVPNAccount 验证 VPN 账号合一 CRUD。
func TestStoreVPNAccount(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hash, err := auth.HashPassword("testpass12")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateVPNAccount(persist.User{
		Username:     "testuser",
		PasswordHash: hash,
		PublicKey:    "dGVzdC1wdWJsaWMta2V5LTEyMzQ1Njc4OTAxMjM0NTY3ODkw",
		VPNIP:        "10.88.0.10",
		AllowedIPs:   []string{"192.168.1.0/24"},
		IPMode:       persist.IPModeFixed,
		Enabled:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetUserByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.VPNIP != "10.88.0.10" {
		t.Fatalf("vpn_ip mismatch: %s", got.VPNIP)
	}
	if !got.HasVPN() {
		t.Fatal("expected vpn identity")
	}
}
