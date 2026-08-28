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

// TestClientValidateRequiresUsername 无 username 时拒绝。
func TestClientValidateRequiresUsername(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Server.TLS.CAFile = "./c.crt"
	if err := cfg.Validate(); err == nil {
		t.Fatal("should require auth.username")
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
