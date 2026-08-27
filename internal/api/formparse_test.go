package api_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestLoginMultipartForm 浏览器 FormData / curl -F 使用 multipart，Go 1.26 ParseForm 不解析此类请求。
func TestLoginMultipartForm(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "multipart-login.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 900, 3600)
	if err := ensureTestAdmin(store, authSvc, "admin", "changeme123"); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN:    config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1"},
		API:    config.APISection{ListenHosts: []string{"127.0.0.1"}, Port: 8080, SessionTTLSec: 3600},
	}
	pool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk-test")

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("username", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteField("password", "changeme123"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("multipart login status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Set-Cookie"); got == "" {
		t.Fatal("expected session cookie")
	}
}
