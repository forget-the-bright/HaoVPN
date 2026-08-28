package api

import (
	"strings"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/persist"
)

// TestBuildClientExportYAML 验证导出为账号密码登录模板，不含私钥与策略字段。
func TestBuildClientExportYAML(t *testing.T) {
	cfg := &config.ServerConfig{
		Server: config.ServerSection{
			Listen: "0.0.0.0:8443",
			TLS:    config.TLSSection{CertFile: "./certs/server.crt"},
		},
		VPN: config.VPNSection{MTU: 1420, GatewayIP: "10.88.0.1"},
	}
	u := &persist.User{
		Username:      "engineer1",
		PrivateKeyEnc: "peer-priv-key-base64",
		PublicKey:     "peer-pub-key-base64",
		VPNIP:         "10.88.0.5",
		AllowedIPs:    []string{"192.168.1.0/24", "10.88.0.0/24"},
	}
	out := buildClientExportYAML(cfg, u, "peer-priv-key-base64", "server-tunnel-pub", "./certs/server.crt")

	checks := []struct {
		name string
		ok   bool
	}{
		{"不含 peer 段", !strings.Contains(out, "\npeer:") && !strings.HasPrefix(strings.TrimSpace(out), "peer:")},
		{"不含 private_key", !strings.Contains(out, `private_key:`)},
		{"不含 vpn_ip（握手下发）", !strings.Contains(out, `vpn_ip:`)},
		{"不含 allowed_ips（握手下发）", !strings.Contains(out, `allowed_ips:`)},
		{"不含 gateway_ip（握手下发）", !strings.Contains(out, `gateway_ip:`)},
		{"listen 0.0.0.0 导出为占位符", strings.Contains(out, `address: "REPLACE_WITH_SERVER_IP:8443"`)},
		{"含 ca_file 路径", strings.Contains(out, `ca_file: "./certs/server.crt"`)},
		{"默认校验 TLS", strings.Contains(out, `insecure_skip_verify: false`)},
		{"不含误导性 127.0.0.1 隧道地址", !strings.Contains(out, `address: "127.0.0.1:8443"`)},
		{"不含 admin 密码", !strings.Contains(out, "changeme")},
		{"含心跳超时", strings.Contains(out, `heartbeat_timeout_sec: 90`)},
		{"含 auth.username", strings.Contains(out, `username: "engineer1"`)},
	}
	for _, c := range checks {
		if !c.ok {
			t.Fatalf("%s 失败，导出内容:\n%s", c.name, out)
		}
	}
}
