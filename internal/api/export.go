package api

import (
	"fmt"
	"strings"

	"haovpn/internal/brand"
	"haovpn/internal/config"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// buildClientExportYAML 生成可下载的客户端 YAML（策略与私钥由握手下发）。
//
// 参数：cfg — 服务端配置（监听地址、MTU）；u — 账号；caFile — TLS CA 路径。
// 返回：带中文注释的 YAML 字符串；不含明文私钥（登录后握手下发）。
func buildClientExportYAML(cfg *config.ServerConfig, u *persist.User, plainPrivateKey, serverPubKey, caFile string) string {
	_ = plainPrivateKey
	_ = serverPubKey
	def := &config.ClientConfig{}
	def.ApplyDefaults()
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
	b.WriteString(fmt.Sprintf("  heartbeat_interval_sec: %d\n", def.Server.HeartbeatIntervalSec))
	b.WriteString(fmt.Sprintf("  heartbeat_timeout_sec: %d\n", def.Server.HeartbeatTimeoutSec))
	b.WriteString(fmt.Sprintf("  dial_timeout_sec: %d\n\n", def.Server.DialTimeoutSec))
	b.WriteString("tun:\n")
	b.WriteString(fmt.Sprintf("  name: %q\n", brand.DefaultTunName))
	mtu := cfg.VPN.MTU
	if mtu <= 0 {
		mtu = netutil.DefaultMTU
	}
	b.WriteString(fmt.Sprintf("  mtu: %d\n\n", mtu))
	b.WriteString("auth:\n")
	b.WriteString(fmt.Sprintf("  username: %q\n", u.Username))
	b.WriteString("  # password: 请用 GUI 输入，或环境变量 HAOVPN_PASSWORD\n\n")
	b.WriteString(fmt.Sprintf("reconnect:\n  initial_sec: %d\n  max_sec: %d\n\n", def.Reconnect.InitialSec, def.Reconnect.MaxSec))
	b.WriteString(fmt.Sprintf("log:\n  level: %q\n  file: %q\n", def.Log.Level, def.Log.File))
	return b.String()
}

// exportServerAddress 将服务端 listen 转为客户端 YAML 中的 address 字段。
//
// 0.0.0.0/:: 监听时替换为 REPLACE_WITH_SERVER_IP 占位符，提示部署者填写公网 IP。
func exportServerAddress(listen string) string {
	host, port, ok := netutil.SplitHostPortLoose(listen)
	if !ok {
		return "REPLACE_WITH_SERVER_IP:8443"
	}
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		return "REPLACE_WITH_SERVER_IP:" + port
	}
	return listen
}
