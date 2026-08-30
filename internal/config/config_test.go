package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/netutil"
)

// TestLoadServerGeneratesDefault 验证首次启动自动生成带注释的 server.yaml
func TestLoadServerGeneratesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	cfg, created, err := config.LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("应生成默认配置")
	}
	if cfg.API.AllowPublicBind {
		t.Fatal("默认 allow_public_bind 应为 false")
	}
	if !cfg.VPN.RequireTun {
		t.Fatal("默认 require_tun 应为 true")
	}
	data, _ := os.ReadFile(path)
	if len(data) == 0 || !contains(string(data), "allow_public_bind") {
		t.Fatal("默认配置应含中文注释与 allow_public_bind 字段")
	}
}

// TestServerWildcardRejected 验证 0.0.0.0 未勾选 allow_public_bind 时拒绝加载
func TestServerWildcardRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	_, _, _ = config.LoadServer(path)
	content := `# test
server:
  listen: "0.0.0.0:8443"
  tls:
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    auto_generate: true
vpn:
  subnet: "10.88.0.0/24"
  gateway_ip: "10.88.0.1"
  mtu: 1420
  heartbeat_timeout_sec: 30
nat:
  enabled: true
  allowed_lan_cidrs: ["192.168.1.0/24"]
database:
  path: "./data/haovpn.db"
api:
  listen_hosts: ["0.0.0.0"]
  port: 8080
  allow_public_bind: false
security:
  tunnel_allowed_source_ips: []
  enforce_split_tunnel: true
admin:
  username: "admin"
  password: "changeme"
log:
  level: "info"
  file: "./logs/server.log"
`
	_ = os.WriteFile(path, []byte(content), 0o600)
	_, _, err := config.LoadServer(path)
	if err == nil {
		t.Fatal("应拒绝 0.0.0.0 且 allow_public_bind=false")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestClientApplyDefaults(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.ApplyDefaults()
	if cfg.Tun.MTU != netutil.DefaultMTU {
		t.Fatalf("mtu=%d", cfg.Tun.MTU)
	}
	if cfg.Server.HeartbeatIntervalSec != 15 {
		t.Fatalf("heartbeat=%d", cfg.Server.HeartbeatIntervalSec)
	}
	if cfg.Server.SendQueueSize != netutil.DefaultSendQueueSize {
		t.Fatalf("queue=%d", cfg.Server.SendQueueSize)
	}
	if cfg.Log.File == "" {
		t.Fatal("log file empty")
	}
}

func TestSendQueueClampAndDisplayTimezoneValidate(t *testing.T) {
	sc := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN:    config.VPNSection{Subnet: "10.88.0.0/24", GatewayIP: "10.88.0.1", SendQueueSize: 99999},
		Database: config.DatabaseSection{Path: "./data/t.db"},
		API:      config.APISection{Port: 8080, DisplayTimezone: "GMT+8"},
		Admin:    config.AdminSection{Username: "admin"},
	}
	if err := sc.Validate(); err != nil {
		t.Fatal(err)
	}
	if sc.VPN.SendQueueSize != netutil.MaxSendQueueSize {
		t.Fatalf("queue clamped=%d", sc.VPN.SendQueueSize)
	}
	sc.API.DisplayTimezone = "Not/Real"
	if err := sc.Validate(); err == nil {
		t.Fatal("非法 display_timezone 应失败")
	}
}

