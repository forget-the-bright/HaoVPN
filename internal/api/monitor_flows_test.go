package api_test

import (
	"encoding/json"
	"net/http"
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
	"net/http/httptest"
	"strings"
)

// TestMonitorFlowsNilEcho Flows 为 nil 时仍回显请求的 limit/offset。
func TestMonitorFlowsNilEcho(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "flows-nil.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	if err := ensureTestAdmin(store, authSvc, "admin", "changeme12"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ServerConfig{
		Server:   config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", MTU: 1420},
		API:      config.APISection{Port: 8080, SessionTTLSec: 3600},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)
	sess := sessionmgr.New(store)
	sess.Flows = nil // 模拟未挂流表
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sess, testVPNService(store, pool, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	client := &http.Client{}
	login, err := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=changeme12"))
	if err != nil {
		t.Fatal(err)
	}
	cookies := login.Cookies()
	login.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/monitor/flows?limit=25&offset=10", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Items  []any `json:"items"`
		Total  int   `json:"total"`
		Limit  int   `json:"limit"`
		Offset int   `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Limit != 25 || out.Offset != 10 || out.Total != 0 || out.Items == nil {
		t.Fatalf("nil Flows 应回显 limit/offset 且空 items: %+v", out)
	}
}
