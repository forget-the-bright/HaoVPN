package api_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
)

// TestAccountPrivateKeyAESAndExport 带 keyEnc 创建账号：库内须 enc:v1:，导出 YAML 可解出明文私钥。
func TestAccountPrivateKeyAESAndExport(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "aes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	keyEnc, err := security.NewKeyEnc(key)
	if err != nil {
		t.Fatal(err)
	}

	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")

	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443", TLS: config.TLSSection{CertFile: filepath.Join(dir, "c.crt")}},
		VPN:    config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", MTU: 1420},
		API:    config.APISection{Port: 8080, SessionTTLSec: 3600},
		Log:    config.LogSection{File: filepath.Join(dir, "server.log")},
	}
	if err := security.EnsureServerCert(cfg.Server.TLS.CertFile, filepath.Join(dir, "c.key"), true, nil); err != nil {
		t.Fatal(err)
	}
	pool, _ := ippool.New(cfg.VPN.Subnet)
	pool.Reserve(cfg.VPN.GatewayIP)

	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), pool, keyEnc, time.Now(), "srv-pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{}
	loginResp, err := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=changeme123"))
	if err != nil {
		t.Fatal(err)
	}
	cookies := loginResp.Cookies()
	loginResp.Body.Close()

	csrf := fetchCSRF(t, client, ts.URL, cookies)

	userReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
		strings.NewReader("username=eng_aes&password=SecurePass123!&ip_mode=fixed"))
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
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(userBody, &userOut)
	if userOut.ID == 0 {
		t.Fatal(string(userBody))
	}

	var enc string
	err = store.DB().QueryRow(`SELECT private_key_enc FROM users WHERE id=?`, userOut.ID).Scan(&enc)
	if err != nil {
		t.Fatal(err)
	}
	if !security.IsEncryptedPrivateKey(enc) {
		t.Fatalf("DB private_key_enc not encrypted: %q", enc[:min(40, len(enc))])
	}
	plain, err := keyEnc.OpenPrivateKey(enc)
	if err != nil || plain == "" {
		t.Fatalf("decrypt stored key: %v", err)
	}
	_ = plain

	exportReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/users/"+itoa64(userOut.ID)+"/export", nil)
	for _, c := range cookies {
		exportReq.AddCookie(c)
	}
	exportResp, err := client.Do(exportReq)
	if err != nil {
		t.Fatal(err)
	}
	exportBody, _ := io.ReadAll(exportResp.Body)
	exportResp.Body.Close()
	if !strings.Contains(string(exportBody), `username:`) {
		t.Fatalf("export missing auth.username:\n%s", exportBody)
	}
	if strings.Contains(string(exportBody), `private_key:`) {
		t.Fatalf("export should not embed private_key")
	}

	zipReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/users/"+itoa64(userOut.ID)+"/export.zip", nil)
	for _, c := range cookies {
		zipReq.AddCookie(c)
	}
	zipResp, err := client.Do(zipReq)
	if err != nil {
		t.Fatal(err)
	}
	zipBytes, _ := io.ReadAll(zipResp.Body)
	zipResp.Body.Close()
	if zipResp.StatusCode != http.StatusOK {
		t.Fatalf("export.zip: %d", zipResp.StatusCode)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["client.yaml"] || !names["README.txt"] {
		t.Fatalf("zip missing files: %v", names)
	}
}

// TestLogsAPIContainsMarker 写日志后 /api/v1/logs 须返回含标记行。
func TestLogsAPIContainsMarker(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")
	_ = os.MkdirAll(dir, 0o755)
	_ = logger.Init(logger.Config{Level: "info", File: logPath, MaxSizeMB: 10, MaxBackups: 1})
	defer logger.Close()
	marker := "FIELD-GATE-LOG-MARKER-XYZ"
	logger.Info("%s", marker)

	store, err := persist.Open(filepath.Join(dir, "logs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme123")

	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN:    config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1"},
		API:    config.APISection{Port: 8080, SessionTTLSec: 3600},
		Log:    config.LogSection{File: logPath},
	}
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), nil, nil, time.Now(), "pk")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{}
	loginResp, _ := client.Post(ts.URL+"/api/v1/login", "application/x-www-form-urlencoded",
		strings.NewReader("username=admin&password=changeme123"))
	cookies := loginResp.Cookies()
	loginResp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/logs?tail=100", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs: %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), marker) {
		t.Fatalf("logs API missing marker: %s", body)
	}
}

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

func itoa64(n int64) string {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
