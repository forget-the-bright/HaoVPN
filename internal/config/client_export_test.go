package config

import (
	"strings"
	"testing"
)

func TestBuildClientExportYAML(t *testing.T) {
	out := BuildClientExportYAML("0.0.0.0:8443", "engineer1", "./certs/server.crt", 1420)
	checks := []struct {
		name string
		ok   bool
	}{
		{"含 username", strings.Contains(out, `username: "engineer1"`)},
		{"无 peer 段", !strings.Contains(out, "\npeer:") && !strings.HasPrefix(out, "peer:")},
		{"无 private_key", !strings.Contains(out, "private_key:")},
		{"无 vpn_ip", !strings.Contains(out, "vpn_ip:")},
		{"占位符 address", strings.Contains(out, `address: "REPLACE_WITH_SERVER_IP:8443"`)},
		{"ca_file", strings.Contains(out, `ca_file: "./certs/server.crt"`)},
		{"心跳", strings.Contains(out, "heartbeat_timeout_sec:")},
	}
	for _, c := range checks {
		if !c.ok {
			t.Fatalf("%s 失败:\n%s", c.name, out)
		}
	}
}

func TestBuildClientExportYAMLDefaultCA(t *testing.T) {
	out := BuildClientExportYAML("192.168.1.1:8443", "u1", "", 0)
	if !strings.Contains(out, `ca_file: "`+DefaultServerCertPath+`"`) {
		t.Fatalf("empty caFile should use default:\n%s", out)
	}
}

func TestResolveServerCertPath(t *testing.T) {
	if got := ResolveServerCertPath(nil); got != DefaultServerCertPath {
		t.Fatalf("nil cfg: got %q", got)
	}
	cfg := &ServerConfig{Server: ServerSection{TLS: TLSSection{CertFile: "/custom.crt"}}}
	if got := ResolveServerCertPath(cfg); got != "/custom.crt" {
		t.Fatalf("custom: got %q", got)
	}
}

func TestExportServerAddress(t *testing.T) {
	if got := ExportServerAddress("192.168.1.10:8443"); got != "192.168.1.10:8443" {
		t.Fatalf("got %q", got)
	}
	if got := ExportServerAddress("0.0.0.0:9443"); got != "REPLACE_WITH_SERVER_IP:9443" {
		t.Fatalf("got %q", got)
	}
}
