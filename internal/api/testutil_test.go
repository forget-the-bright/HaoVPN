package api_test

import (
	"encoding/json"
	"io"
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
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/vpnaccount"
)

// ensureTestAdmin 创建测试用 admin 并清除须改密标记（避免 must_change 阻塞 API 测试）。
func ensureTestAdmin(store *persist.Store, authSvc *auth.Service, username, password string) error {
	if err := authSvc.EnsureAdmin(username, password, false); err != nil {
		return err
	}
	u, err := store.GetUserByUsername(username)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return store.UpdateUserPassword(u.ID, hash, true)
}

// testVPNService 构造 api.NewServer 所需的 vpnaccount.Service（测试用）。
func testVPNService(store *persist.Store, pool *ippool.Pool, cfg *config.ServerConfig) *vpnaccount.Service {
	return &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
}

// testVPNServiceWithSessions 构造带 OnKickUser 回调的 Service（禁号/改策略测试用）。
func testVPNServiceWithSessions(store *persist.Store, pool *ippool.Pool, cfg *config.ServerConfig, sess *sessionmgr.Manager) *vpnaccount.Service {
	svc := testVPNService(store, pool, cfg)
	if sess != nil {
		svc.OnKickUser = sess.KickUser
	}
	return svc
}

// newTestAPI 启动 httptest 服务并完成 admin 登录 + CSRF 获取（manual_ip 等测试共用）。
func newTestAPI(t *testing.T) (*httptest.Server, *http.Client, []*http.Cookie, string, *persist.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")
	cfg := &config.ServerConfig{
		Server:   config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", MTU: 1420},
		API:      config.APISection{Port: 8080, SessionTTLSec: 3600},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	client := &http.Client{}
	login, err := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=changeme123"))
	if err != nil {
		t.Fatal(err)
	}
	cookies := login.Cookies()
	login.Body.Close()
	csrf := fetchCSRF(t, client, ts.URL, cookies)
	return ts, client, cookies, csrf, store
}

// fetchCSRF 从已登录会话获取 CSRF token。
func fetchCSRF(t *testing.T, client *http.Client, base string, cookies []*http.Cookie) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/csrf", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var out map[string]string
	_ = json.Unmarshal(b, &out)
	if out["csrf_token"] == "" {
		t.Fatal(string(b))
	}
	return out["csrf_token"]
}
