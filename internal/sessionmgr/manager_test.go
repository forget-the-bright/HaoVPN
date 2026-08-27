package sessionmgr_test

import (
	"path/filepath"
	"testing"

	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestValidateVPNAccess 验证禁用账号时拒绝准入。
func TestValidateVPNAccess(t *testing.T) {
	mgr := sessionmgr.New(nil)
	user := &persist.User{Enabled: false, PublicKey: "pk"}
	if err := mgr.ValidateVPNAccess(user); err == nil {
		t.Fatal("禁用账号应拒绝")
	}
	user.Enabled = true
	if err := mgr.ValidateVPNAccess(user); err != nil {
		t.Fatalf("启用账号应通过: %v", err)
	}
}

// TestOnlineCount 初始在线数应为 0。
func TestOnlineCount(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	mgr := sessionmgr.New(store)
	if mgr.OnlineCount() != 0 {
		t.Fatal("expected 0 online")
	}
}
