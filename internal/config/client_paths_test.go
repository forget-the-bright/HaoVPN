package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/config"
)

// TestResolveFilePathOrder 绝对 > exe 存在 > 配置目录。
func TestResolveFilePathOrder(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	cfgDir := filepath.Join(root, "cfg")
	if err := os.MkdirAll(filepath.Join(exeDir, "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfgDir, "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	exeCA := filepath.Join(exeDir, "certs", "ca.crt")
	cfgCA := filepath.Join(cfgDir, "certs", "ca.crt")
	if err := os.WriteFile(exeCA, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgCA, []byte("cfg"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := config.ResolveFilePath(exeDir, cfgDir, "./certs/ca.crt")
	if filepath.Clean(got) != filepath.Clean(exeCA) {
		t.Fatalf("应优先 exe 旁: got=%q want=%q", got, exeCA)
	}

	// 删掉 exe 侧 → 回退配置目录
	_ = os.Remove(exeCA)
	got = config.ResolveFilePath(exeDir, cfgDir, "./certs/ca.crt")
	if filepath.Clean(got) != filepath.Clean(cfgCA) {
		t.Fatalf("应回退配置目录: got=%q want=%q", got, cfgCA)
	}

	// 两边都不存在 → 仍回退配置目录（便于新建 log）
	_ = os.Remove(cfgCA)
	got = config.ResolveFilePath(exeDir, cfgDir, "./logs/x.log")
	want := filepath.Clean(filepath.Join(cfgDir, "logs", "x.log"))
	if filepath.Clean(got) != want {
		t.Fatalf("两边不存在应落配置目录: got=%q want=%q", got, want)
	}

	abs, err := filepath.Abs(filepath.Join(root, "abs.crt"))
	if err != nil {
		t.Fatal(err)
	}
	got = config.ResolveFilePath(exeDir, cfgDir, abs)
	if filepath.Clean(got) != filepath.Clean(abs) {
		t.Fatalf("绝对路径应原样: got=%q", got)
	}
}

// TestLoadClientResolvesRelativePathsAgainstConfigDir
// 模拟自启：CWD≠配置目录；CA 仅在 yaml 旁时仍能解析到配置目录。
func TestLoadClientResolvesRelativePathsAgainstConfigDir(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "app")
	certs := filepath.Join(cfgDir, "certs")
	if err := os.MkdirAll(certs, 0o755); err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(certs, "ca.crt")
	if err := os.WriteFile(caPath, []byte("dummy-ca"), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(cfgDir, "client.yaml")
	body := `
server:
  address: "127.0.0.1:8443"
  tls:
    ca_file: "./certs/ca.crt"
auth:
  username: "u1"
log:
  level: "info"
  file: "./logs/client.log"
`
	if err := os.WriteFile(yamlPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(root, "cwd")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cfg, _, err := config.LoadClient(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	// 测试二进制所在目录通常没有 ./certs/ca.crt → 回退 yaml 目录
	wantCA := filepath.Clean(caPath)
	if filepath.Clean(cfg.Server.TLS.CAFile) != wantCA {
		t.Fatalf("ca_file=%q want %q", cfg.Server.TLS.CAFile, wantCA)
	}
	wantLog := filepath.Clean(filepath.Join(cfgDir, "logs", "client.log"))
	if filepath.Clean(cfg.Log.File) != wantLog {
		t.Fatalf("log.file=%q want %q", cfg.Log.File, wantLog)
	}
	raw, _ := os.ReadFile(yamlPath)
	if !strings.Contains(string(raw), "./certs/ca.crt") {
		t.Fatalf("磁盘 YAML 应保留相对 ca_file，got:\n%s", raw)
	}
}

// TestResolveRelativePathsKeepsAbsolute 绝对路径不被改写。
func TestResolveRelativePathsKeepsAbsolute(t *testing.T) {
	cfg := &config.ClientConfig{}
	base := t.TempDir()
	absCA, err := filepath.Abs(filepath.Join(base, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	absLog, err := filepath.Abs(filepath.Join(base, "c.log"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.TLS.CAFile = absCA
	cfg.Log.File = absLog
	cfg.ResolveRelativePaths(filepath.Join(t.TempDir(), "other", "client.yaml"))
	if cfg.Server.TLS.CAFile != absCA || cfg.Log.File != absLog {
		t.Fatalf("绝对路径被改写: ca=%q log=%q", cfg.Server.TLS.CAFile, cfg.Log.File)
	}
}

// TestLoadServerResolvesRelativePaths 服务端同样走共用解析。
func TestLoadServerResolvesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	certs := filepath.Join(dir, "certs")
	if err := os.MkdirAll(certs, 0o755); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(certs, "server.crt")
	key := filepath.Join(certs, "server.key")
	_ = os.WriteFile(cert, []byte("c"), 0o600)
	_ = os.WriteFile(key, []byte("k"), 0o600)
	path := filepath.Join(dir, "server.yaml")
	body := `
server:
  listen: "127.0.0.1:8443"
  tls:
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    auto_generate: false
vpn:
  subnet: "10.88.0.0/24"
  gateway_ip: "10.88.0.1"
  mtu: 1420
  heartbeat_timeout_sec: 30
nat:
  enabled: false
database:
  path: "./data/haovpn.db"
api:
  listen_hosts: ["127.0.0.1"]
  port: 8080
  allow_public_bind: false
admin:
  username: "admin"
  password: "changeme"
log:
  level: "info"
  file: "./logs/server.log"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cfg.Server.TLS.CertFile) != filepath.Clean(cert) {
		t.Fatalf("cert=%q want %q", cfg.Server.TLS.CertFile, cert)
	}
	if filepath.Clean(cfg.Database.Path) != filepath.Clean(filepath.Join(dir, "data", "haovpn.db")) {
		t.Fatalf("db=%q", cfg.Database.Path)
	}
}
