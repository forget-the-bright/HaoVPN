package config_test

import (
	"testing"

	"haovpn/internal/config"
)

// TestClientValidateAuthUsername 仅 auth.username 即可通过（密码可交互补齐）。
func TestClientValidateAuthUsername(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Auth.Username = "engineer1"
	cfg.Server.TLS.CAFile = "./certs/server.crt"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestClientValidateRejectsPrivateKeyOnly 仅私钥不再作为合法配置。
func TestClientValidateRejectsPrivateKeyOnly(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Peer.PrivateKey = "dGVzdA=="
	cfg.Server.TLS.CAFile = "./c.crt"
	if err := cfg.Validate(); err == nil {
		t.Fatal("should require auth.username")
	}
}

// TestClientValidateRejectsFullTunnel 残留 0.0.0.0/0 仍拒绝。
func TestClientValidateRejectsFullTunnel(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Auth.Username = "u"
	cfg.Server.TLS.CAFile = "./c.crt"
	cfg.Peer.AllowedIPs = []string{"0.0.0.0/0"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("should reject full tunnel")
	}
}

// TestClientValidateRequiresCA 未 skip 时须配置 ca_file。
func TestClientValidateRequiresCA(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Auth.Username = "u"
	if err := cfg.Validate(); err == nil {
		t.Fatal("should require ca_file")
	}
}

// TestPreferGateway 握手网关优先于 yaml。
func TestPreferGateway(t *testing.T) {
	p := &config.ClientPeerSection{GatewayIP: "10.88.0.9"}
	if config.PreferGateway("10.88.0.1", "10.88.0.50", p) != "10.88.0.1" {
		t.Fatal("handshake gateway should win")
	}
	if config.PreferGateway("", "10.99.0.50", p) != "10.88.0.9" {
		t.Fatal("yaml fallback")
	}
}
