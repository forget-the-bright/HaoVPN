package config_test

import (
	"strings"
	"testing"

	"haovpn/internal/config"
)

// TestValidateTunNameOK 合法名（默认品牌名）须通过。
func TestValidateTunNameOK(t *testing.T) {
	for _, name := range []string{"haovpn0", "Haovpn_1", "a", strings.Repeat("x", 64)} {
		if err := config.ValidateTunName(name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
}

// TestValidateTunNameReject 空、过长、含正则/路径危险字符须拒绝。
func TestValidateTunNameReject(t *testing.T) {
	for _, name := range []string{"", " ", "haovpn 0", "haovpn.0", "a|b", "../x", "a'b", strings.Repeat("y", 65)} {
		if err := config.ValidateTunName(name); err == nil {
			t.Fatalf("%q should be rejected", name)
		}
	}
}

// TestClientValidateRejectsBadTunName Validate 须拦截非法 tun.name。
func TestClientValidateRejectsBadTunName(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "192.168.1.10:8443"
	cfg.Auth.Username = "engineer1"
	cfg.Server.TLS.CAFile = "./certs/server.crt"
	cfg.Tun.Name = "bad|name"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected tun.name validation error")
	}
}
