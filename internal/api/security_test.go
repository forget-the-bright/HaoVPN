package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestCSRFBlocksPOST 无 CSRF token 的 POST 应 403（安全清单 #9）。
func TestCSRFBlocksPOST(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "csrf.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")
	token, _, _ := authSvc.Login("admin", "changeme123", "127.0.0.1")
	cfg := &config.ServerConfig{Server: config.ServerSection{Listen: "127.0.0.1:8443"}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), nil, nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// TestLoginLockout 连续错误密码应锁定（安全清单 #4）。
func TestLoginLockout(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "lock.db"))
	defer store.Close()
	authSvc := auth.New(store, 3, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")
	for i := 0; i < 3; i++ {
		_, _, _ = authSvc.Login("admin", "wrong", "10.0.0.1")
	}
	_, _, err := authSvc.Login("admin", "changeme123", "10.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "稍后再试") {
		t.Fatalf("expected lockout: %v", err)
	}
}
