package auth_test

import (
	"errors"
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestChangePasswordRequiresOldAndRevokesSessions 自改密须校验旧密码，并由 LogoutAllForUser 清会话。
func TestChangePasswordRequiresOldAndRevokesSessions(t *testing.T) {
	store, err := persist.Open(filepath.Join(t.TempDir(), "chpwd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := auth.New(store, 5, 60, 3600)
	if err := svc.EnsureAdmin("admin", "OldPass123!", true); err != nil {
		t.Fatal(err)
	}
	tok, u, err := svc.Login("admin", "OldPass123!", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.ValidateSession(tok); !ok {
		t.Fatal("session should be valid")
	}

	if err := svc.ChangePassword(u.ID, "wrong-old", "NewPass123!"); !errors.Is(err, auth.ErrWrongOldPassword) {
		t.Fatalf("want ErrWrongOldPassword, got %v", err)
	}
	if err := svc.ChangePassword(u.ID, "OldPass123!", "NewPass123!"); err != nil {
		t.Fatal(err)
	}
	n := svc.LogoutAllForUser(u.ID)
	if n < 1 {
		t.Fatalf("expected revoke >=1, got %d", n)
	}
	if _, ok := svc.ValidateSession(tok); ok {
		t.Fatal("session should be revoked after LogoutAllForUser")
	}
	if _, _, err := svc.Login("admin", "NewPass123!", "127.0.0.1"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}
