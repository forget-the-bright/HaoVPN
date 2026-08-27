package api

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"

	"haovpn/internal/config"
	"haovpn/internal/persist"
)

// buildAccountExportZip 打包 client.yaml + server.crt + README；缺证书则失败。
func buildAccountExportZip(cfg *config.ServerConfig, u *persist.User, plainPrivateKey, serverPubKey string) ([]byte, error) {
	certPath := cfg.Server.TLS.CertFile
	if certPath == "" {
		certPath = "./certs/server.crt"
	}
	pem, err := os.ReadFile(certPath)
	if err != nil || len(pem) == 0 {
		return nil, fmt.Errorf("导出失败: 找不到 TLS 证书 %s（请确认服务端已生成证书）", certPath)
	}

	yaml := buildClientExportYAML(cfg, u, plainPrivateKey, serverPubKey, "./certs/server.crt")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create("client.yaml")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write([]byte(yaml)); err != nil {
		return nil, err
	}

	readme := `# HaoVPN 客户端配置包
# 1. 解压到同一目录（须含 certs/server.crt）
# 2. 将 client.yaml 中 server.address 改为客户端可达的服务端 IP
# 3. 运行 GUI 或: haovpn-client -c client.yaml
# 注意：默认校验 TLS（insecure_skip_verify: false）；vpn_ip/allowed_ips 由握手下发
`
	rw, err := zw.Create("README.txt")
	if err != nil {
		return nil, err
	}
	if _, err := rw.Write([]byte(readme)); err != nil {
		return nil, err
	}

	cw, err := zw.Create("certs/server.crt")
	if err != nil {
		return nil, err
	}
	if _, err := cw.Write(pem); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
