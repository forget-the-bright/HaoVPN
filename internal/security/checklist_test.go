package security_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/logger"
	"haovpn/internal/security"
)

func init() { _ = logger.Init(logger.Config{Level: "error"}) }

// TestSecurityChecklist 覆盖 meta-plan 安全测试清单（可单元测试部分）。
func TestSecurityChecklist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")

	// 1. 0.0.0.0 + allow_public_bind=false → 拒绝加载
	writeYAML(path, wildcardYAML(false))
	_, _, err := config.LoadServer(path)
	if err == nil {
		t.Fatal("应拒绝 0.0.0.0 且 allow_public_bind=false")
	}

	// 2. 0.0.0.0 + allow_public_bind=true → BindCheck 通过
	if err := security.BindCheck([]string{"0.0.0.0"}, true); err != nil {
		t.Fatal(err)
	}

	// 8. 客户端 yaml 含 legacy peer 段仍可加载（策略由握手下发，peer 已废弃）
	clientPath := filepath.Join(dir, "client.yaml")
	writeYAML(clientPath, clientLegacyPeerYAML())
	_, _, err = config.LoadClient(clientPath)
	if err != nil {
		t.Fatalf("legacy peer 应被忽略: %v", err)
	}

	// 7. 日志脱敏
	red := security.Redact("password=secret123 private_key=abc")
	if strings.Contains(red, "secret123") {
		t.Fatal("密码应被脱敏")
	}
}

func writeYAML(path, content string) {
	_ = os.WriteFile(path, []byte(content), 0o600)
}

func wildcardYAML(allowPublic bool) string {
	ap := "false"
	if allowPublic {
		ap = "true"
	}
	return `
server:
  listen: "0.0.0.0:8443"
  tls: { cert_file: "./c.crt", key_file: "./c.key", auto_generate: true }
vpn: { subnet: "10.88.0.0/24", gateway_ip: "10.88.0.1", mtu: 1420, heartbeat_timeout_sec: 30 }
nat: { enabled: false, allowed_lan_cidrs: [] }
database: { path: "./data.db" }
api: { listen_hosts: ["0.0.0.0"], port: 8080, allow_public_bind: ` + ap + ` }
security: { enforce_split_tunnel: true }
admin: { username: "admin", password: "changeme123" }
log: { level: "info", file: "./s.log" }
`
}

func clientLegacyPeerYAML() string {
	return `
server: { address: "127.0.0.1:8443", tls: { ca_file: "./certs/server.crt", insecure_skip_verify: false } }
auth: { username: "eng" }
tun: { name: "t0", mtu: 1420 }
peer:
  allowed_ips: ["0.0.0.0/0"]
reconnect: { initial_sec: 1, max_sec: 8 }
log: { level: "info", file: "./c.log" }
`
}
