package auth_test

import (
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestLoginAndLockout 验证登录成功与密码哈希（step5）。
func TestLoginAndLockout(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := auth.New(store, 3, 60, 3600)
	_ = svc.EnsureAdmin("admin", "changeme123", false)

	token, user, err := svc.Login("admin", "changeme123", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "admin" {
		t.Fatalf("username=%s", user.Username)
	}
	se, ok := svc.ValidateSession(token)
	if !ok {
		t.Fatal("session invalid")
	}
	if se.UserID != user.ID {
		t.Fatal("user id mismatch")
	}

	for i := 0; i < 3; i++ {
		_, _, err := svc.Login("admin", "wrongpass", "192.168.1.5")
		if err == nil {
			t.Fatal("wrong password should fail")
		}
	}
	_, _, err = svc.Login("admin", "changeme123", "192.168.1.5")
	if err == nil {
		t.Fatal("expected lockout after 3 failures")
	}
	if !strings.Contains(err.Error(), "稍后再试") {
		t.Fatalf("lockout error message: %v", err)
	}
}

// TestEnsureAdminSyncPasswordFromConfig yaml 同步密码（home 开发）。
func TestEnsureAdminSyncPasswordFromConfig(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := auth.New(store, 5, 60, 3600)
	if err := svc.EnsureAdmin("admin", "oldpass123", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureAdmin("admin", "newpass123", true); err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Login("admin", "newpass123", "10.0.0.1")
	if err != nil {
		t.Fatalf("synced password login: %v", err)
	}
	_, _, err = svc.Login("admin", "oldpass123", "10.0.0.2")
	if err == nil {
		t.Fatal("old password should fail")
	}
}

// TestEnsureAdminFreshSyncSkipsMustChange 首启且 sync_password_from_config 时不应强制改密（acceptance/home）。
func TestEnsureAdminFreshSyncSkipsMustChange(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "fresh-sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := auth.New(store, 5, 60, 3600)
	if err := svc.EnsureAdmin("admin", "pass12345", true); err != nil {
		t.Fatal(err)
	}
	u, err := store.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if u.MustChangePassword {
		t.Fatal("sync 首启 admin 不应须改密")
	}
}

// TestEnsureAdminSyncClearsMustChangePassword sync 时应清除须改密标记（避免每次重启卡在改密页）。
func TestEnsureAdminSyncClearsMustChangePassword(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "sync-must.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := auth.New(store, 5, 60, 3600)
	if err := svc.EnsureAdmin("admin", "pass12345", false); err != nil {
		t.Fatal(err)
	}
	u, err := store.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if !u.MustChangePassword {
		t.Fatal("新建 admin 应须改密")
	}
	if err := svc.EnsureAdmin("admin", "pass12345", true); err != nil {
		t.Fatal(err)
	}
	u, err = store.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if u.MustChangePassword {
		t.Fatal("sync_password_from_config 后不应再强制改密")
	}
	_, user, err := svc.Login("admin", "pass12345", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if user.MustChangePassword {
		t.Fatal("登录响应不应要求改密")
	}
}

// TestVerifyTunnelLoginRejectsMustChangePassword 须改密账号不得隧道登录。
func TestVerifyTunnelLoginRejectsMustChangePassword(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "tunnel-must.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := auth.New(store, 5, 60, 3600)

	hash, err := auth.HashPassword("SecurePass123!")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateVPNAccount(persist.User{
		Username:           "eng1",
		PasswordHash:       hash,
		MustChangePassword: true,
		PublicKey:          "pk-test",
		PrivateKeyEnc:      "sk-test",
		VPNIP:              "10.88.0.10",
		AllowedIPs:         []string{"192.168.1.0/24"},
		IPMode:             persist.IPModeFixed,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.VerifyTunnelLogin("eng1", "SecurePass123!", "10.0.0.8")
	if err == nil {
		t.Fatal("须改密时应拒绝隧道登录")
	}
	if !strings.Contains(err.Error(), "修改密码") {
		t.Fatalf("错误应提示改密: %v", err)
	}

	u, err := store.GetUserByUsername("eng1")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserPassword(u.ID, hash, true); err != nil {
		t.Fatal(err)
	}
	got, err := svc.VerifyTunnelLogin("eng1", "SecurePass123!", "10.0.0.8")
	if err != nil {
		t.Fatalf("清除须改密后应可登录: %v", err)
	}
	if got.Username != "eng1" {
		t.Fatalf("username=%s", got.Username)
	}
}
