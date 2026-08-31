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
	"haovpn/internal/probedefense"
	"haovpn/internal/readmodel"
	"haovpn/internal/sessionmgr"
)

// TestRequireAuthInjectsSessionContext requireAuth 后 handler 可从 context 取 actor，无需再解析 Cookie。
func TestRequireAuthInjectsSessionContext(t *testing.T) {
	store, _ := persist.Open(filepath.Join(t.TempDir(), "ctx.db"))
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := &config.ServerConfig{Server: config.ServerSection{Listen: "127.0.0.1:8443"}, API: config.APISection{Port: 8080}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, nil, cfg), nil, time.Now(), "pk")
	srv.SetProbeGuard(probedefense.New(store, probedefense.DefaultConfig()))

	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	csrf := authSvc.GetCSRF(token)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/security/blocks", strings.NewReader(`{"ip":"198.51.100.77","reason":"ctx-test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("manual ban via requireAuth: %d %s", w.Code, w.Body.String())
	}
	logs, _, err := store.ListAuditLogsFiltered(readmodel.AuditListFilter{Action: "probe_ban_manual", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("expected audit log from actorFromRequest via session context")
	}
}
