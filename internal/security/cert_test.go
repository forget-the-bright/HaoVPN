package security_test

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"haovpn/internal/security"
)

// TestEnsureServerCert 验证自签证书自动生成。
func TestEnsureServerCert(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	if err := security.EnsureServerCert(cert, key, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := security.EnsureServerCert(cert, key, true, nil); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureServerCertSAN 新生成证书应包含 listen 与 cert_sans 中的 IP。
func TestEnsureServerCertSAN(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	opts := &security.CertGenOptions{
		ListenAddr: "192.168.10.5:8443",
		CertSANs:   []string{"vpn.example.com", "10.0.0.8"},
	}
	if err := security.EnsureServerCert(certPath, keyPath, true, opts); err != nil {
		t.Fatal(err)
	}
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("no pem block")
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	wantIPs := map[string]bool{"127.0.0.1": true, "192.168.10.5": true, "10.0.0.8": true}
	for _, ip := range c.IPAddresses {
		delete(wantIPs, ip.String())
	}
	if len(wantIPs) != 0 {
		t.Fatalf("missing IPs in cert SAN: %v", wantIPs)
	}
	foundDNS := false
	for _, d := range c.DNSNames {
		if d == "localhost" || d == "vpn.example.com" {
			foundDNS = true
		}
	}
	if !foundDNS {
		t.Fatalf("dns names=%v", c.DNSNames)
	}
}

// TestSelfSignedAsClientCA 自签证书须能作为客户端 RootCAs 校验自身（公司机 ca_file 路径）。
func TestSelfSignedAsClientCA(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	opts := &security.CertGenOptions{CertSANs: []string{"192.168.196.17"}}
	if err := security.EnsureServerCert(certPath, keyPath, true, opts); err != nil {
		t.Fatal(err)
	}
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !leaf.IsCA {
		t.Fatal("自签信任根须 IsCA=true")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		t.Fatal("append ca")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Fatalf("自签作 CA 校验失败: %v", err)
	}
}
