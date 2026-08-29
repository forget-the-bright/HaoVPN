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
)

// TestAcceptanceAPIFlow 模拟验收：登录 → 建 VPN 账号 → 导出 → 审计 → 改策略踢线。
func TestAcceptanceAPIFlow(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "accept.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 900, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")

	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443", TLS: config.TLSSection{CertFile: filepath.Join(dir, "c.crt"), KeyFile: filepath.Join(dir, "c.key"), AutoGenerate: true}},
		VPN:    config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", MTU: 1420},
		NAT:    config.NATSection{Enabled: false},
		API:    config.APISection{ListenHosts: []string{"127.0.0.1"}, Port: 8080, SessionTTLSec: 3600},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
	pool, err := ippool.New(cfg.VPN.Subnet)
	if err != nil {
		t.Fatal(err)
	}
	pool.Reserve(cfg.VPN.GatewayIP)

	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "server-pub-key-test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 未登录导出 → 401
	resp, err := http.Get(ts.URL + "/api/v1/users/1/export")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("export without auth: got %d", resp.StatusCode)
	}

	client := &http.Client{}
	loginResp, err := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=changeme123"))
	if err != nil {
		t.Fatal(err)
	}
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", loginResp.StatusCode)
	}
	cookies := loginResp.Cookies()

	csrfReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/csrf", nil)
	for _, c := range cookies {
		csrfReq.AddCookie(c)
	}
	csrfResp, err := client.Do(csrfReq)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(csrfResp.Body)
	csrfResp.Body.Close()
	var csrfOut map[string]string
	if err := json.Unmarshal(body, &csrfOut); err != nil || csrfOut["csrf_token"] == "" {
		t.Fatalf("csrf token: %s", body)
	}
	csrf := csrfOut["csrf_token"]

	// 创建 VPN 账号（一步：用户+密钥+IP）
	userReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
		strings.NewReader("username=engineer1&password=SecurePass123!&ip_mode=fixed"))
	userReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	userReq.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		userReq.AddCookie(c)
	}
	userResp, err := client.Do(userReq)
	if err != nil {
		t.Fatal(err)
	}
	userBody, _ := io.ReadAll(userResp.Body)
	userResp.Body.Close()
	if userResp.StatusCode != http.StatusOK {
		t.Fatalf("create user: %d %s", userResp.StatusCode, userBody)
	}
	var userOut struct {
		ID     int64  `json:"id"`
		VPNIP  string `json:"vpn_ip"`
		IPMode string `json:"ip_mode"`
	}
	if err := json.Unmarshal(userBody, &userOut); err != nil || userOut.ID == 0 {
		t.Fatalf("user id: %s", userBody)
	}
	if userOut.VPNIP == "" {
		t.Fatal("fixed mode should allocate vpn_ip")
	}
	userID := userOut.ID

	// 导出配置
	exportReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/users/"+itoa(userID)+"/export", nil)
	for _, c := range cookies {
		exportReq.AddCookie(c)
	}
	exportResp, err := client.Do(exportReq)
	if err != nil {
		t.Fatal(err)
	}
	exportBody, _ := io.ReadAll(exportResp.Body)
	exportResp.Body.Close()
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("export: %d", exportResp.StatusCode)
	}
	if !strings.Contains(string(exportBody), "username:") {
		t.Fatal("export yaml missing auth.username")
	}
	if strings.Contains(string(exportBody), "private_key:") {
		t.Fatal("export yaml should not embed private_key")
	}
	if strings.Contains(string(exportBody), "changeme123") {
		t.Fatal("export should not contain admin password")
	}

	// 审计：account_create + config_export
	auditReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/audit", nil)
	for _, c := range cookies {
		auditReq.AddCookie(c)
	}
	auditResp, err := client.Do(auditReq)
	if err != nil {
		t.Fatal(err)
	}
	auditBody, _ := io.ReadAll(auditResp.Body)
	auditResp.Body.Close()
	ab := string(auditBody)
	if !strings.Contains(ab, "account_create") || !strings.Contains(ab, "config_export") {
		t.Fatalf("audit missing actions: %s", ab)
	}

	// PATCH 策略
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/users/"+itoa(userID)+"/vpn",
		strings.NewReader(`{"allowed_ips":["192.168.31.0/24"]}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		patchReq.AddCookie(c)
	}
	patchResp, err := client.Do(patchReq)
	if err != nil {
		t.Fatal(err)
	}
	patchBody, _ := io.ReadAll(patchResp.Body)
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch vpn: %d %s", patchResp.StatusCode, patchBody)
	}

	// WebUI 主页面（/peers 重定向到 /users）
	for _, path := range []string{"/", "/users", "/connections", "/audit", "/login"} {
		pageReq, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		if path != "/login" {
			for _, c := range cookies {
				pageReq.AddCookie(c)
			}
		}
		pageResp, err := client.Do(pageReq)
		if err != nil {
			t.Fatal(err)
		}
		pageResp.Body.Close()
		if pageResp.StatusCode != http.StatusOK {
			t.Fatalf("page %s: %d", path, pageResp.StatusCode)
		}
	}
	// /peers 托管路由页（原重定向 /users；现独立 Managed Routes）
	peersReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/peers", nil)
	for _, c := range cookies {
		peersReq.AddCookie(c)
	}
	peersResp, err := client.Do(peersReq)
	if err != nil {
		t.Fatal(err)
	}
	peersBody, _ := io.ReadAll(peersResp.Body)
	peersResp.Body.Close()
	if peersResp.StatusCode != http.StatusOK {
		t.Fatalf("/peers: %d", peersResp.StatusCode)
	}
	if !strings.Contains(string(peersBody), "托管路由") {
		t.Fatal("/peers 页面应含托管路由文案")
	}

	if exportResp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header on export")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
