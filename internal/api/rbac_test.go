package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestWebLoginNonAdminForbidden 非管理账号不得登录 Web。
func TestWebLoginNonAdminForbidden(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/rbac.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("engpass123")
	_, err = store.CreateVPNAccount(persist.User{
		Username: "engineer", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.20", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.ServerConfig{API: config.APISection{Port: 8080}}
	srv := api.NewServer(cfg, store, auth.New(store, 5, 60, 3600), audit.New(store),
		sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	body := "username=engineer&password=engpass123"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-admin, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestWebLoginAdminOK 管理员可登录 Web。
func TestWebLoginAdminOK(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/admin.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	if err := authSvc.EnsureAdmin("admin", "adminpass12", false); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ServerConfig{API: config.APISection{Port: 8080, SessionTTLSec: 3600}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store),
		sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	body := "username=admin&password=adminpass12"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestMustChangePasswordBlocksAPI 须改密时除改密/登出外 API 返回 403。
func TestMustChangePasswordBlocksAPI(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/mcp.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	if err := authSvc.EnsureAdmin("admin", "adminpass12", false); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ServerConfig{API: config.APISection{Port: 8080, SessionTTLSec: 3600}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store),
		sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")

	body := "username=admin&password=adminpass12"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()
	var session string
	for _, c := range cookie {
		if c.Name == "session" {
			session = c.Value
		}
	}
	if session == "" {
		t.Fatal("no session cookie")
	}

	reqDash := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	reqDash.AddCookie(&http.Cookie{Name: "session", Value: session})
	wDash := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wDash, reqDash)
	if wDash.Code != http.StatusForbidden {
		t.Fatalf("dashboard want 403, got %d body=%s", wDash.Code, wDash.Body.String())
	}
}