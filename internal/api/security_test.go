package api_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/readmodel"
	"haovpn/internal/sessionmgr"
)

// TestCSRFBlocksPOST 无 CSRF token 的 POST 应 403（安全清单 #9）。
func TestCSRFBlocksPOST(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "csrf.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	cfg := &config.ServerConfig{Server: config.ServerSection{Listen: "127.0.0.1:8443"}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// TestLoginLockout 连续 5 次错误密码应锁定并写 audit（meta-plan #4）。
func TestLoginLockout(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "lock.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	auditLog := audit.New(store)
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, LoginMaxAttempts: 5},
	}
	srv := api.NewServer(cfg, store, authSvc, auditLog, sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	clientIP := "10.0.0.1"
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/login", strings.NewReader("username=admin&password=wrong"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = clientIP + ":1234"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/login", strings.NewReader("username=admin&password=SecureAdmin123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = clientIP + ":1234"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected lockout 401, got %d", resp.StatusCode)
	}

	logs, _, err := store.ListAuditLogsFiltered(readmodel.AuditListFilter{Action: "login_failed", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) < 5 {
		t.Fatalf("expected >=5 login_failed audit, got %d", len(logs))
	}
}

// TestSessionCookieSecure secure_cookies 时 Session Cookie 带 Secure。
func TestSessionCookieSecure(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "cookie.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SecureCookies: true, SessionTTLSec: 3600},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader("username=admin&password=SecureAdmin123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var secure bool
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" && c.Secure {
			secure = true
		}
	}
	if !secure {
		t.Fatal("expected Secure flag on session cookie")
	}
}

// TestSessionCookieHttpOnlySameSite 登录 Cookie 须 HttpOnly 且 SameSite=Lax（meta-plan S3 补全）。
func TestSessionCookieHttpOnlySameSite(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "cookie2.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SessionTTLSec: 3600},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader("username=admin&password=SecureAdmin123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var httpOnly bool
	var sameSiteLax bool
	for _, c := range w.Result().Cookies() {
		if c.Name != "session" {
			continue
		}
		httpOnly = c.HttpOnly
		sameSiteLax = c.SameSite == http.SameSiteLaxMode
	}
	if !httpOnly {
		t.Fatal("expected HttpOnly on session cookie")
	}
	if !sameSiteLax {
		t.Fatal("expected SameSite=Lax on session cookie")
	}
}

// TestPasswordResetBadForm 畸形表单重置密码应返回 400（A1 回归）。
func TestPasswordResetBadForm(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "resetform.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{Server: config.ServerSection{Listen: "127.0.0.1:8443"}, API: config.APISection{Port: 8080}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	csrf := authSvc.GetCSRF(token)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/1/password", strings.NewReader("%"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad form, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestLogsAPIRedactsSensitive /api/v1/logs 返回前须脱敏敏感字段（S1 端到端）。
func TestLogsAPIRedactsSensitive(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")
	const secret = "SuperSecretPass123"
	if err := os.WriteFile(logPath, []byte("login password="+secret+" ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, _ := persist.Open(filepath.Join(dir, "logs.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SessionTTLSec: 3600},
		Log:    config.LogSection{File: logPath},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/logs?source=file", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logs API: %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("logs API 仍含明文密码: %s", body)
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Fatalf("logs API 应含 [REDACTED]: %s", body)
	}
}
