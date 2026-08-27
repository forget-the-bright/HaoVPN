package api

import (
	"fmt"
	"strings"

	"haovpn/internal/config"
	"haovpn/internal/persist"
)

// buildClientExportYAML 生成可下载的客户端 YAML（策略与私钥由握手下发）。
func buildClientExportYAML(cfg *config.ServerConfig, u *persist.User, plainPrivateKey, serverPubKey, caFile string) string {
	_ = plainPrivateKey
	_ = serverPubKey
	var b strings.Builder
	b.WriteString("# HaoVPN 客户端配置（由服务端导出）\n")
	b.WriteString("# 使用 GUI/CLI 以账号密码登录；vpn_ip/gateway/allowed_ips/私钥均由握手下发\n")
	b.WriteString("# 请将 server.address 改成客户端可达的服务端 IP\n\n")
	b.WriteString("server:\n")
	addr := exportServerAddress(cfg.Server.Listen)
	b.WriteString(fmt.Sprintf("  address: %q\n", addr))
	b.WriteString("  tls:\n")
	if caFile == "" {
		caFile = cfg.Server.TLS.CertFile
	}
	if caFile == "" {
		caFile = "./certs/server.crt"
	}
	b.WriteString(fmt.Sprintf("    ca_file: %q\n", caFile))
	b.WriteString("    insecure_skip_verify: false\n")
	b.WriteString("  heartbeat_interval_sec: 15\n")
	b.WriteString("  heartbeat_timeout_sec: 90\n")
	b.WriteString("  dial_timeout_sec: 3\n\n")
	b.WriteString("tun:\n")
	b.WriteString("  name: \"haovpn0\"\n")
	b.WriteString(fmt.Sprintf("  mtu: %d\n\n", cfg.VPN.MTU))
	b.WriteString("auth:\n")
	b.WriteString(fmt.Sprintf("  username: %q\n", u.Username))
	b.WriteString("  # password: 请用 GUI 输入，或环境变量 HAOVPN_PASSWORD\n\n")
	b.WriteString("reconnect:\n  initial_sec: 1\n  max_sec: 3\n\n")
	b.WriteString("log:\n  level: \"info\"\n  file: \"./logs/client.log\"\n")
	return b.String()
}

func exportServerAddress(listen string) string {
	host, port, ok := splitHostPortLoose(listen)
	if !ok {
		return "REPLACE_WITH_SERVER_IP:8443"
	}
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		return "REPLACE_WITH_SERVER_IP:" + port
	}
	return listen
}

func splitHostPortLoose(addr string) (host, port string, ok bool) {
	if i := strings.LastIndex(addr, ":"); i > 0 && i < len(addr)-1 {
		return addr[:i], addr[i+1:], true
	}
	return "", "", false
}
