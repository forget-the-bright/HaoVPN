package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestDashboardRequiresAuth 未登录访问管理 API 应返回 401（安全测试清单 #5）。
func TestDashboardRequiresAuth(t *testing.T) {
	cfg := &config.ServerConfig{
		Server: config.ServerSection{
			Listen: "127.0.0.1:8443",
			TLS:    config.TLSSection{CertFile: "./certs/server.crt"},
		},
		API: config.APISection{Port: 8080},
	}
	store, err := persist.Open(t.TempDir() + "/api.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	srv := api.NewServer(cfg, store, auth.New(store, 5, 60, 3600), audit.New(store),
		sessionmgr.New(store), nil, nil, time.Now(), "test-server-pk")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", w.Code, w.Body.String())
	}
}
