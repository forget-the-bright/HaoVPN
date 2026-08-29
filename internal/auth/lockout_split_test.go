package auth_test

import (
	"errors"
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestWebAndTunnelLockoutsIndependent VPN 喷洒不得锁死 Web 管理口（分表锁定）。
func TestWebAndTunnelLockoutsIndependent(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "lock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := auth.New(store, 3, 60, 3600)
	if err := svc.EnsureAdmin("admin", "AdminPass12", true); err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("VpnPass12!")
	_, err = store.CreateVPNAccount(persist.User{
		Username: "eng", PasswordHash: hash, PublicKey: "pk", PrivateKeyEnc: "sk",
		VPNIP: "10.88.0.9", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ip := "203.0.113.50"
	for i := 0; i < 3; i++ {
		_, err := svc.VerifyTunnelLogin("eng", "wrong", ip)
		if err == nil {
			t.Fatal("tunnel bad password should fail")
		}
	}
	_, err = svc.VerifyTunnelLogin("eng", "wrong", ip)
	if !errors.Is(err, auth.ErrLoginLocked) {
		t.Fatalf("tunnel should lock: %v", err)
	}
	// Web 同 IP 仍可登录
	if _, _, err := svc.Login("admin", "AdminPass12", ip); err != nil {
		t.Fatalf("web must not share tunnel lockout: %v", err)
	}
}
