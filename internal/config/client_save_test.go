package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/config"
)

func TestSaveClientRememberPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	seed := `# 现场注释
server:
  address: "127.0.0.1:8443"
  tls:
    ca_file: "./certs/ca.crt"
    insecure_skip_verify: true
auth:
  username: "user1"
peer:
  private_key: ""
security:
  kill_switch: true
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ClientConfig{
		Server: config.ClientServerSection{
			Address: "127.0.0.1:8443",
			TLS:     config.ClientTLSSection{InsecureSkipVerify: true},
		},
		Auth: config.ClientAuthSection{
			Username:         "user1",
			RememberPassword: true,
			Password:         "SecretPass123!",
		},
		Security: config.ClientSecuritySection{KillSwitch: true},
	}
	if err := config.SaveClient(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "现场注释") {
		t.Fatalf("comment lost: %q", s)
	}
	if strings.Contains(s, "peer:") {
		t.Fatalf("peer section should be removed: %q", s)
	}
	if !strings.Contains(s, "remember_password: true") {
		t.Fatalf("missing remember_password: true in %q", s)
	}
	if !strings.Contains(s, "SecretPass123!") {
		t.Fatalf("missing password in %q", s)
	}
	if !strings.Contains(s, "kill_switch: true") {
		t.Fatalf("security section should be preserved: %q", s)
	}

	cfg.GUI.AutoConnect = true
	cfg.GUI.StartMinimized = true
	cfg.Auth.RememberPassword = true
	cfg.Auth.Password = "SecretPass123!"
	if err := config.SaveClient(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if !strings.Contains(s, "auto_connect: true") || !strings.Contains(s, "start_minimized: true") {
		t.Fatalf("gui flags missing: %q", s)
	}

	cfg.Auth.RememberPassword = false
	if err := config.SaveClient(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if strings.Contains(s, "SecretPass123!") {
		t.Fatalf("password should be cleared when remember false: %q", s)
	}
}

func TestCanAutoConnect(t *testing.T) {
	cfg := &config.ClientConfig{
		GUI:  config.ClientGUISection{AutoConnect: true},
		Auth: config.ClientAuthSection{RememberPassword: true, Password: "x"},
	}
	if !cfg.CanAutoConnect() {
		t.Fatal("expected CanAutoConnect")
	}
	cfg.Auth.Password = ""
	if cfg.CanAutoConnect() {
		t.Fatal("empty password should block")
	}
	cfg.Auth.Password = "x"
	cfg.Auth.RememberPassword = false
	if cfg.CanAutoConnect() {
		t.Fatal("remember false should block")
	}
}

func TestSaveClientResolveAuthRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	seed := `server:
  address: "host.example:8443"
  tls:
    ca_file: "./certs/ca.crt"
    insecure_skip_verify: true
auth:
  username: "eng"
security:
  kill_switch: true
`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.ClientConfig{
		Server: config.ClientServerSection{
			Address: "host.example:8443",
			TLS:     config.ClientTLSSection{InsecureSkipVerify: true},
		},
		Auth: config.ClientAuthSection{
			Username:         "eng",
			RememberPassword: true,
			Password:         "MyPass1234!",
		},
		Security: config.ClientSecuritySection{KillSwitch: true},
	}
	if err := config.SaveClient(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.LoadClient(path)
	if err != nil {
		t.Fatal(err)
	}
	user, pass := loaded.ResolveAuth()
	if user != "eng" || pass != "MyPass1234!" {
		t.Fatalf("ResolveAuth got %q / %q", user, pass)
	}
	if !loaded.Security.KillSwitch {
		t.Fatal("kill_switch should persist")
	}
	if !loaded.Auth.RememberPassword {
		t.Fatal("remember_password should persist")
	}
}

func TestLoadClientIgnoresLegacyPeer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.yaml")
	content := `
server:
  address: "127.0.0.1:8443"
  tls:
    ca_file: "./certs/ca.crt"
    insecure_skip_verify: true
auth:
  username: "u1"
peer:
  allowed_ips: ["0.0.0.0/0"]
  gateway_ip: "10.88.0.9"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.LoadClient(path)
	if err != nil {
		t.Fatalf("legacy peer should be ignored: %v", err)
	}
	if cfg.Auth.Username != "u1" {
		t.Fatalf("username=%q", cfg.Auth.Username)
	}
}
