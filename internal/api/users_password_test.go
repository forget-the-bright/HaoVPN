package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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
	"haovpn/internal/sessionmgr"
)

// TestAdminResetUserPassword 管理员可重置 VPN 账号密码；旧密码失效、新密码可用。
func TestAdminResetUserPassword(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "pwd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	if err := ensureTestAdmin(store, authSvc, "admin", "adminpass12"); err != nil {
		t.Fatal(err)
	}

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	oldHash, _ := auth.HashPassword("OldPass123!")
	u, err := store.CreateVPNAccount(persist.User{
		Username: "engineer1", PasswordHash: oldHash, PublicKey: kp.PublicKey, PrivateKeyEnc: kp.PrivateKey,
		VPNIP: "10.88.0.30", IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	userID := u

	cfg := &config.ServerConfig{
		VPN: config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1"},
		API: config.APISection{Port: 8080, SessionTTLSec: 3600},
	}
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store),
		testVPNService(store, pool, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{}
	loginResp, err := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=adminpass12"))
	if err != nil {
		t.Fatal(err)
	}
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", loginResp.StatusCode)
	}
	csrf := fetchCSRFToken(t, client, ts.URL, loginResp.Cookies())

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users/"+strconv.FormatInt(userID, 10)+"/password",
		strings.NewReader("new_password=NewPass123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range loginResp.Cookies() {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset password: %d %s", resp.StatusCode, b)
	}

	u2, err := store.GetUserByID(userID)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(u2.PasswordHash, "NewPass123!") {
		t.Fatal("new password not stored")
	}
	if auth.CheckPassword(u2.PasswordHash, "OldPass123!") {
		t.Fatal("old password still works")
	}
}

// TestAdminResetUserPasswordShort 密码过短返回 400。
func TestAdminResetUserPasswordShort(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/pwdshort.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "adminpass12")
	admin, _ := store.GetUserByUsername("admin")

	cfg := &config.ServerConfig{API: config.APISection{Port: 8080, SessionTTLSec: 3600}}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store),
		testVPNService(store, nil, cfg), nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{}
	loginResp, _ := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=adminpass12"))
	csrf := fetchCSRFToken(t, client, ts.URL, loginResp.Cookies())

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users/"+strconv.FormatInt(admin.ID, 10)+"/password",
		strings.NewReader("new_password=short"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range loginResp.Cookies() {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func fetchCSRFToken(t *testing.T, client *http.Client, base string, cookies []*http.Cookie) string {
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
	if err := json.Unmarshal(b, &out); err != nil || out["csrf_token"] == "" {
		t.Fatalf("csrf: %s", b)
	}
	return out["csrf_token"]
}
