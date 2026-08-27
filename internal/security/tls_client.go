package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"haovpn/internal/config"
	"haovpn/internal/netutil"
)

// ClientTLSBuildOptions 构建客户端 TLS 的输入（与 config 解耦，便于测试）。
//
// 字段：
//   CAFile — 信任根 PEM 路径；非 insecure 模式下必填。
//   InsecureSkipVerify — true 时跳过证书校验（仅开发/内网）；CA 读失败不阻断。
//   ServerName — TLS SNI/主机名校验名；空时由 ServerAddress 推导。
//   ServerAddress — client.yaml server.address；用于 resolveClientServerName 回落。
type ClientTLSBuildOptions struct {
	CAFile             string
	InsecureSkipVerify bool
	ServerName         string
	ServerAddress      string
}

// BuildClientTLSFromOptions 从 TLS 选项构造客户端 *tls.Config。
//
// 参数：opts — CAFile/InsecureSkipVerify/ServerName/ServerAddress 见类型注释。
// 返回：*tls.Config 含 RootCAs 与 ServerName；err 为非 skip 模式下 CA 缺失或 PEM 无效。
// 副作用：无；不发起网络连接。
// 并发：返回的配置可多 goroutine 只读使用。
func BuildClientTLSFromOptions(opts ClientTLSBuildOptions) (*tls.Config, error) {
	rootCAs, err := loadClientRootCAs(opts.CAFile, opts.InsecureSkipVerify)
	if err != nil {
		return nil, err
	}
	tlsCfg := ClientTLSConfigWithRootCAs(rootCAs, opts.InsecureSkipVerify)
	tlsCfg.ServerName = resolveClientServerName(opts.ServerName, opts.ServerAddress)
	return tlsCfg, nil
}

// BuildClientTLS 构造客户端 TLS 配置；CA 无效时返回错误（不静默回退）。
//
// 参数：cfg — 非 nil 的 ClientConfig；读取 cfg.Server.TLS 与 cfg.Server.Address。
// 返回：*tls.Config；err 同 BuildClientTLSFromOptions。
// 副作用：无。
// 并发：clientapp 拨号前调用；配置只读。
func BuildClientTLS(cfg *config.ClientConfig) (*tls.Config, error) {
	return BuildClientTLSFromOptions(ClientTLSBuildOptions{
		CAFile:             cfg.Server.TLS.CAFile,
		InsecureSkipVerify: cfg.Server.TLS.InsecureSkipVerify,
		ServerName:         cfg.Server.TLS.ServerName,
		ServerAddress:      cfg.Server.Address,
	})
}

func loadClientRootCAs(caFile string, insecureSkipVerify bool) (*x509.CertPool, error) {
	ca := strings.TrimSpace(caFile)
	if !insecureSkipVerify {
		if ca == "" {
			return nil, fmt.Errorf("未配置 server.tls.ca_file 且未启用 insecure_skip_verify")
		}
		pool, err := loadPEMCertPool(ca)
		if err != nil {
			return nil, err
		}
		return pool, nil
	}
	if ca == "" {
		return nil, nil
	}
	pool, err := loadPEMCertPool(ca)
	if err != nil {
		return nil, nil // skip verify 模式下 CA 读失败不阻断
	}
	return pool, nil
}

func loadPEMCertPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 CA 失败 %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA 文件不是有效 PEM: %s", path)
	}
	return pool, nil
}

func resolveClientServerName(serverName, serverAddress string) string {
	if strings.TrimSpace(serverName) != "" {
		return serverName
	}
	host := netutil.HostFromAddr(serverAddress)
	if host != "" && host != "0.0.0.0" && !strings.HasPrefix(host, "REPLACE_") {
		return host
	}
	return "localhost"
}
