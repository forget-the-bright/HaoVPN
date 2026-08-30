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
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestLoginPageLoadsExternalLoginScript 登录页引用 static/login.js；CSP 仍允许 unsafe-inline（其它页残留内联）。
func TestLoginPageLoadsExternalLoginScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "loginForm") || !strings.Contains(body, "/static/login.js") {
		t.Fatal("登录页应含表单并引用 /static/login.js")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") || !strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("CSP 仍须允许 unsafe-inline（其它管理页内联未迁完）: %q", csp)
	}
}
