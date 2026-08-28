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
	"haovpn/internal/crypto"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
)

// TestSecurityChecklistMetaPlan 用 HTTP + SQLite 审计验证 meta-plan 安全清单（可自动化项）。
func TestSecurityChecklistMetaPlan(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "sec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// #5 未登录导出 → 401
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "SecureAdmin123!")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "tunnel-pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/users/1/export")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("#5 未登录导出: 期望 401，得 %d", resp.StatusCode)
	}

	// #9 CSRF：已登录但无 token 的 POST → 403
	token, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("#9 CSRF: 期望 403，得 %d body=%s", w.Code, w.Body.String())
	}

	// #2 allow_public_bind 审计记录
	auditLog := audit.New(store)
	api.LogPublicBindAudit(auditLog)
	logs, err := store.ListAuditLogs(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	foundAudit := false
	for _, e := range logs {
		if e.Action == "management_public_bind_enabled" {
			foundAudit = true
			break
		}
	}
	if !foundAudit {
		t.Fatal("#2 public_bind 应写入 audit_logs")
	}

	// #6 禁用账号 → KickUser（user_id）
	sessMgr := sessionmgr.New(store)
	var kickedUser int64
	sessMgr.SetKickHandler(func(id int64) { kickedUser = id })
	vpnSvcKick := testVPNService(store, pool, cfg)
	vpnSvcKick.OnKickUser = sessMgr.KickUser
	srvKick := api.NewServer(cfg, store, authSvc, audit.New(store), sessMgr, vpnSvcKick, nil, time.Now(), "tunnel-pk")

	hash, _ := auth.HashPassword("UserPass1234!")
	kp, _ := crypto.GenerateKeyPair()
	uid, err := store.CreateVPNAccount(persist.User{
		Username: "u_disable", PasswordHash: hash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.10", AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	token2, _, _ := authSvc.Login("admin", "SecureAdmin123!", "127.0.0.1")
	csrf := authSvc.GetCSRF(token2)
	disableReq, _ := http.NewRequest(http.MethodPost, "/api/v1/users/"+itoa(uid), strings.NewReader("action=disable"))
	disableReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	disableReq.Header.Set("X-CSRF-Token", csrf)
	disableReq.AddCookie(&http.Cookie{Name: "session", Value: token2})
	w2 := httptest.NewRecorder()
	srvKick.Handler().ServeHTTP(w2, disableReq)
	if w2.Code != http.StatusOK {
		t.Fatalf("#6 禁用用户: %d %s", w2.Code, w2.Body.String())
	}
	if kickedUser != uid {
		t.Fatalf("#6 禁用用户应 KickUser(%d)，实际 kicked=%d", uid, kickedUser)
	}
	u, _ := store.GetUserByID(uid)
	if u.Enabled {
		t.Fatal("#6 用户应已禁用")
	}

	// #7 日志脱敏
	red := security.Redact("login password=SecretPass private_key=abc123")
	if strings.Contains(red, "SecretPass") || strings.Contains(red, "abc123") {
		t.Fatal("#7 Redact 仍含敏感明文")
	}
}

func testServerCfg() *config.ServerConfig {
	return &config.ServerConfig{
		Server: config.ServerSection{
			Listen: "127.0.0.1:8443",
			TLS:    config.TLSSection{CertFile: "./c.crt", KeyFile: "./c.key", AutoGenerate: true},
		},
		VPN:      config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", MTU: 1420},
		NAT:      config.NATSection{Enabled: false},
		API:      config.APISection{ListenHosts: []string{"127.0.0.1"}, Port: 8080, SessionTTLSec: 3600},
		Security: config.SecuritySection{EnforceSplitTunnel: true},
	}
}
