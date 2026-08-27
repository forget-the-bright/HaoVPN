package api

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/persist"
	"haovpn/internal/security"
)

// TestBuildAccountExportZip 有证书时 zip 含 yaml 与 certs/server.crt。
func TestBuildAccountExportZip(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	if err := security.EnsureServerCert(cert, key, true, nil); err != nil {
		t.Fatal(err)
	}
	cfg := &config.ServerConfig{
		Server: config.ServerSection{
			Listen: "10.0.0.1:8443",
			TLS:    config.TLSSection{CertFile: cert},
		},
		VPN: config.VPNSection{MTU: 1420, GatewayIP: "10.88.0.1"},
	}
	u := &persist.User{
		Username:   "eng1",
		PublicKey:  "pub",
		VPNIP:      "10.88.0.6",
		AllowedIPs: []string{"10.88.0.0/24"},
	}
	raw, err := buildAccountExportZip(cfg, u, "plain-priv", "server-pk")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "client.yaml" {
			rc, _ := f.Open()
			var b bytes.Buffer
			_, _ = b.ReadFrom(rc)
			_ = rc.Close()
			if !strings.Contains(b.String(), `username: "eng1"`) {
				t.Fatal("zip client.yaml missing auth.username")
			}
			if strings.Contains(b.String(), `private_key:`) {
				t.Fatal("zip client.yaml should not contain private_key")
			}
			if !strings.Contains(b.String(), `insecure_skip_verify: false`) {
				t.Fatal("should default to verify TLS")
			}
			if strings.Contains(b.String(), "insecure_skip_verify\n") && strings.Contains(b.String(), "请用") {
				t.Fatal("must not suggest skip verify")
			}
		}
	}
	if !names["client.yaml"] || !names["README.txt"] || !names["certs/server.crt"] {
		t.Fatalf("zip missing required files: %v", names)
	}
}

// TestBuildAccountExportZipMissingCert 缺证书须失败。
func TestBuildAccountExportZipMissingCert(t *testing.T) {
	cfg := &config.ServerConfig{
		Server: config.ServerSection{
			Listen: "10.0.0.1:8443",
			TLS:    config.TLSSection{CertFile: filepath.Join(t.TempDir(), "no.crt")},
		},
		VPN: config.VPNSection{MTU: 1420},
	}
	_, err := buildAccountExportZip(cfg, &persist.User{Username: "u"}, "", "")
	if err == nil {
		t.Fatal("expected error when cert missing")
	}
	if !strings.Contains(err.Error(), "找不到 TLS 证书") {
		t.Fatalf("err=%v", err)
	}
	_ = os.ErrNotExist
}
