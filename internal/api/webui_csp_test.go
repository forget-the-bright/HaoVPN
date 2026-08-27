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

// TestLoginPageCSPAllowsInlineScript 验证登录页响应头允许内联 JS（白屏根因回归）。
func TestLoginPageCSPAllowsInlineScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), pool, nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "loginForm") || !strings.Contains(body, "<script>") {
		t.Fatal("登录页应含表单与内联脚本")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") || !strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("CSP 未允许内联脚本，浏览器会白屏/登录无效: %q", csp)
	}
}
