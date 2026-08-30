package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestLogoutClearsCookieWithSecureAttributes
// secure_cookies 登录后 logout 清除 Cookie 须带 Secure+SameSite，否则浏览器删不掉。
func TestLogoutClearsCookieWithSecureAttributes(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "logout-cookie.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SecureCookies: true, SessionTTLSec: 3600},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	csrf := authSvc.GetCSRF(token)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	var cleared *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name != "session" {
			continue
		}
		// requireAuth 会先重发滑动 Cookie，logout 再清除；取 MaxAge<0 的那条。
		if c.MaxAge < 0 || c.Value == "" {
			cleared = c
		}
	}
	if cleared == nil {
		t.Fatal("logout 应 Set-Cookie 清除 session（MaxAge=-1）")
	}
	if !cleared.Secure {
		t.Fatal("secure_cookies 下清除 Cookie 须 Secure=true")
	}
	if cleared.SameSite != http.SameSiteLaxMode {
		t.Fatalf("清除 Cookie SameSite 应为 Lax, got %v", cleared.SameSite)
	}
	if !cleared.HttpOnly {
		t.Fatal("清除 Cookie 须 HttpOnly")
	}
}

// TestAuthTouchReissuesSessionCookie 鉴权成功后应重发 session Cookie 刷新 MaxAge。
func TestAuthTouchReissuesSessionCookie(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "touch-cookie.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SessionTTLSec: 7200},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/csrf", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("csrf: %d %s", w.Code, w.Body.String())
	}
	var reissued *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "session" && c.Value == token {
			reissued = c
			break
		}
	}
	if reissued == nil {
		t.Fatal("鉴权后应重发同 token 的 session Cookie")
	}
	if reissued.MaxAge != 7200 {
		t.Fatalf("重发 Cookie MaxAge 应为 SessionTTLSec=7200, got %d", reissued.MaxAge)
	}
}

// TestMustChangePasswordAllowsCSRF 须改密时仍可 GET /api/v1/csrf 刷新 token。
func TestMustChangePasswordAllowsCSRF(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "mustchange-csrf.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	if err := authSvc.EnsureAdmin("admin", "SecureAdmin123!", false); err != nil {
		t.Fatal(err)
	}
	// EnsureAdmin 默认 must_change；勿调用 ensureTestAdmin 清标记。
	token, user, err := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !user.MustChangePassword {
		t.Fatal("预期须改密")
	}
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		API:    config.APISection{Port: 8080, SessionTTLSec: 3600},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/csrf", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("须改密应放行 GET csrf, got %d %s", w.Code, w.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["csrf_token"] == "" {
		t.Fatal("csrf_token 为空")
	}

	// 其它需鉴权 API 仍应 403（公开 health 不经 requireAuth）
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req3.AddCookie(&http.Cookie{Name: "session", Value: token})
	w3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("须改密访问 dashboard 应 403, got %d body=%s", w3.Code, w3.Body.String())
	}
}
