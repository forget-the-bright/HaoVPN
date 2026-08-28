package config

import (
	"fmt"
	"strings"

	"haovpn/internal/brand"
	"haovpn/internal/netutil"
)

// BuildClientExportYAML 生成服务端导出的客户端 YAML（与 clientYAMLTemplate 字段对齐）。
//
// 为何放在 config：避免 api 手写拼装与 defaults 模板漂移；GUI SaveClient 仍用 Node patch（保留注释）。
// 参数：
//   serverListen — 服务端 tunnel listen（如 0.0.0.0:8443）；公网绑定时导出为占位符地址；
//   username — 预填 auth.username；
//   caFile — TLS CA 路径；空则用 "./certs/server.crt"；
//   mtu — TUN MTU；≤0 时用 netutil.DefaultMTU。
// 返回：带中文注释的 YAML；不含 peer/私钥/vpn_ip（均由握手下发）。
func BuildClientExportYAML(serverListen, username, caFile string, mtu int) string {
	def := &ClientConfig{}
	def.ApplyDefaults()
	if caFile == "" {
		caFile = DefaultServerCertPath
	}
	if mtu <= 0 {
		mtu = netutil.DefaultMTU
	}
	var b strings.Builder
	b.WriteString("# HaoVPN 客户端配置（由服务端导出）\n")
	b.WriteString("# 使用 GUI/CLI 以账号密码登录；vpn_ip/gateway/allowed_ips/私钥均由握手下发\n")
	b.WriteString("# 请将 server.address 改成客户端可达的服务端 IP\n\n")
	b.WriteString("server:\n")
	b.WriteString(fmt.Sprintf("  address: %q\n", ExportServerAddress(serverListen)))
	b.WriteString("  tls:\n")
	b.WriteString(fmt.Sprintf("    ca_file: %q\n", caFile))
	b.WriteString("    insecure_skip_verify: false\n")
	b.WriteString(fmt.Sprintf("  heartbeat_interval_sec: %d\n", def.Server.HeartbeatIntervalSec))
	b.WriteString(fmt.Sprintf("  heartbeat_timeout_sec: %d\n", def.Server.HeartbeatTimeoutSec))
	b.WriteString(fmt.Sprintf("  dial_timeout_sec: %d\n\n", def.Server.DialTimeoutSec))
	b.WriteString("tun:\n")
	b.WriteString(fmt.Sprintf("  name: %q\n", brand.DefaultTunName))
	b.WriteString(fmt.Sprintf("  mtu: %d\n\n", mtu))
	b.WriteString("auth:\n")
	b.WriteString(fmt.Sprintf("  username: %q\n", username))
	b.WriteString("  # password: 请用 GUI 输入，或环境变量 HAOVPN_PASSWORD\n\n")
	b.WriteString(fmt.Sprintf("reconnect:\n  initial_sec: %d\n  max_sec: %d\n\n", def.Reconnect.InitialSec, def.Reconnect.MaxSec))
	b.WriteString(fmt.Sprintf("log:\n  level: %q\n  file: %q\n", def.Log.Level, def.Log.File))
	return b.String()
}

// ExportServerAddress 将服务端 listen 转为客户端 YAML 中的 address 字段。
//
// 0.0.0.0/:: 监听时替换为 REPLACE_WITH_SERVER_IP 占位符，提示部署者填写可达 IP。
func ExportServerAddress(listen string) string {
	host, port, ok := netutil.SplitHostPortLoose(listen)
	if !ok {
		return "REPLACE_WITH_SERVER_IP:8443"
	}
	if host == "0.0.0.0" || host == "::" || host == "[::]" {
		return "REPLACE_WITH_SERVER_IP:" + port
	}
	return listen
}
