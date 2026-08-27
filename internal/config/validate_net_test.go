package config_test

import (
	"testing"

	"haovpn/internal/config"
)

func TestValidateGatewayInSubnet(t *testing.T) {
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN: config.VPNSection{
			Subnet:    "10.88.0.0/24",
			GatewayIP: "10.99.0.1",
		},
		Database: config.DatabaseSection{Path: "./data/db"},
		API:      config.APISection{Port: 8080, ListenHosts: []string{"127.0.0.1"}},
		Admin:    config.AdminSection{Username: "admin"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("gateway 不在 subnet 内应失败")
	}
}

func TestValidateInvalidNATCIDR(t *testing.T) {
	cfg := &config.ServerConfig{
		Server: config.ServerSection{Listen: "127.0.0.1:8443"},
		VPN: config.VPNSection{
			Subnet:    "10.88.0.0/24",
			GatewayIP: "10.88.0.1",
		},
		NAT:      config.NATSection{AllowedLANCIDRs: []string{"not-a-cidr"}},
		Database: config.DatabaseSection{Path: "./data/db"},
		API:      config.APISection{Port: 8080, ListenHosts: []string{"127.0.0.1"}},
		Admin:    config.AdminSection{Username: "admin"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("无效 NAT CIDR 应失败")
	}
}
