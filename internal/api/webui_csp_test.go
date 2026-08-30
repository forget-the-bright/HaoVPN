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

// TestLoginPageLoadsExternalLoginScript 登录页引用 static/login.js；CSP script-src 不得含 unsafe-inline。
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
	if !strings.Contains(csp, "script-src") {
		t.Fatalf("CSP 须含 script-src: %q", csp)
	}
	// 只检查 script-src 段：不得含 unsafe-inline（style-src 仍可保留）
	scriptPart := csp
	if idx := strings.Index(csp, "script-src"); idx >= 0 {
		scriptPart = csp[idx:]
		if end := strings.Index(scriptPart, ";"); end >= 0 {
			scriptPart = scriptPart[:end]
		}
	}
	if strings.Contains(scriptPart, "'unsafe-inline'") {
		t.Fatalf("CSP script-src 不得含 unsafe-inline: %q", csp)
	}
}

// TestIndexPageLoadsExternalIndexScript 仪表盘引用 /static/index.js，且无内联 <script>。
func TestIndexPageLoadsExternalIndexScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp-index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader("username=admin&password=changeme12"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginW.Code, loginW.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range loginW.Result().Cookies() {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "/static/index.js") {
		t.Fatal("仪表盘应引用 /static/index.js")
	}
	if strings.Contains(body, "<script>") {
		t.Fatal("仪表盘不应再含内联 <script> 块")
	}
}
