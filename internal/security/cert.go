// cert.go 负责 TLS 自签证书自动生成（开箱即用，与 tls_client.go 服务端加载配合）。
package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"

	"haovpn/internal/fileutil"
	"haovpn/internal/logger"
)

// CertGenOptions 自签证书 SAN 生成选项（仅新建证书时生效）。
//
// 字段：
//   ListenAddr — server.listen 地址；用于推导绑定 IP 或主机名加入 SAN。
//   CertSANs — 配置额外 SAN 列表；元素可为 IP 或 DNS 名，由 buildCertSANs 解析。
type CertGenOptions struct {
	ListenAddr string   // server.listen，用于推导绑定 IP
	CertSANs   []string // 配置额外 SAN
}

// EnsureServerCert 若证书不存在且 autoGenerate 为 true，生成 10 年自签证书到指定路径。
//
// 参数：certFile/keyFile — PEM 路径；autoGenerate — false 且文件缺失时返回错误；
// opts — 可选 SAN 推导参数，nil 时仅 localhost/127.0.0.1。
// 返回：err 为目录创建失败、生成失败或 auto_generate=false 且文件不存在。
// 副作用：可能写入 cert/key PEM 文件（权限 0600）；打 Warn/Info 日志。
// 并发：启动时单线程调用；并发生成同一文件应由调用方避免。
func EnsureServerCert(certFile, keyFile string, autoGenerate bool, opts *CertGenOptions) error {
	if fileExists(certFile) && fileExists(keyFile) {
		return nil
	}
	if !autoGenerate {
		return fmt.Errorf("TLS 证书不存在且 auto_generate=false: %s", certFile)
	}
	if err := fileutil.EnsureParentDir(certFile, 0o755); err != nil {
		return fmt.Errorf("创建证书目录: %w", err)
	}
	logger.Warn("TLS 证书不存在，正在生成 10 年自签证书（生产环境请替换为正式证书）")
	if err := generateSelfSigned(certFile, keyFile, opts); err != nil {
		return err
	}
	logger.Info("自签证书已生成: %s / %s", certFile, keyFile)
	return nil
}

// LoadServerTLS 加载或确保存在后加载服务端 TLS 证书。
//
// 参数：certFile/keyFile/autoGenerate/opts — 同 EnsureServerCert。
// 返回：tls.Certificate 可直接用于 tls.Listen；err 为 Ensure 或 LoadX509KeyPair 失败。
// 副作用：可能触发自签证书生成（见 EnsureServerCert）。
// 并发：启动时单线程调用。
func LoadServerTLS(certFile, keyFile string, autoGenerate bool, opts *CertGenOptions) (tls.Certificate, error) {
	if err := EnsureServerCert(certFile, keyFile, autoGenerate, opts); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(certFile, keyFile)
}

func buildCertSANs(opts *CertGenOptions) (dnsNames []string, ipAddrs []net.IP) {
	seenDNS := map[string]bool{}
	seenIP := map[string]bool{}
	addDNS := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seenDNS[s] {
			return
		}
		seenDNS[s] = true
		dnsNames = append(dnsNames, s)
	}
	addIP := func(ip net.IP) {
		if ip == nil || ip.IsUnspecified() {
			return
		}
		s := ip.String()
		if seenIP[s] {
			return
		}
		seenIP[s] = true
		ipAddrs = append(ipAddrs, ip)
	}

	addDNS("localhost")
	addIP(net.ParseIP("127.0.0.1"))

	if opts != nil {
		for _, s := range opts.CertSANs {
			if ip := net.ParseIP(s); ip != nil {
				addIP(ip)
			} else {
				addDNS(s)
			}
		}
		if opts.ListenAddr != "" {
			host, _, err := net.SplitHostPort(opts.ListenAddr)
			if err == nil {
				if ip := net.ParseIP(host); ip != nil {
					addIP(ip)
				} else if host != "" && host != "0.0.0.0" && host != "::" {
					addDNS(host)
				}
			}
		}
	}
	return dnsNames, ipAddrs
}

// generateSelfSigned 生成 ECDSA P-256 自签证书并写入 PEM 文件。
func generateSelfSigned(certFile, keyFile string, opts *CertGenOptions) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	dnsNames, ipAddrs := buildCertSANs(opts)
	now := time.Now()
	// 自签证书同时作为服务端叶证书与客户端 ca_file 信任根，须带 CA 约束，
	// 否则 Go x509 校验报 parent certificate cannot sign this kind of certificate。
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "HaoVPN-self-signed"},
		NotBefore:             now,
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}
	certOut, err := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	return pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
