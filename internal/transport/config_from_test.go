package transport_test

import (
	"testing"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/netutil"
	"haovpn/internal/transport"
)

func TestFromClientConfigDefaults(t *testing.T) {
	cfg := &config.ClientConfig{}
	tcfg := transport.FromClientConfig(cfg)
	if tcfg.HeartbeatInterval != 15*time.Second {
		t.Fatalf("interval=%v", tcfg.HeartbeatInterval)
	}
	if tcfg.MTU != netutil.DefaultMTU {
		t.Fatalf("mtu=%d", tcfg.MTU)
	}
}

func TestFromClientConfigPartialOverride(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.HeartbeatIntervalSec = 30
	cfg.Tun.MTU = 1280
	cfg.Server.SendQueueSize = 1024
	tcfg := transport.FromClientConfig(cfg)
	if tcfg.HeartbeatInterval != 30*time.Second {
		t.Fatalf("interval=%v", tcfg.HeartbeatInterval)
	}
	if tcfg.MTU != 1280 {
		t.Fatalf("mtu=%d", tcfg.MTU)
	}
	if tcfg.MaxQueueSize != 1024 {
		t.Fatalf("queue=%d", tcfg.MaxQueueSize)
	}
}

func TestFromServerVPNAfterApplyDefaults(t *testing.T) {
	vpn := config.VPNSection{}
	sc := &config.ServerConfig{VPN: vpn}
	sc.ApplyDefaults()
	tcfg := transport.FromServerVPN(sc.VPN)
	if tcfg.HeartbeatTimeout != 90*time.Second {
		t.Fatalf("timeout=%v", tcfg.HeartbeatTimeout)
	}
	if tcfg.MaxQueueSize != netutil.DefaultSendQueueSize {
		t.Fatalf("queue=%d", tcfg.MaxQueueSize)
	}
	if sc.API.DisplayTimezone != "UTC" {
		t.Fatalf("tz=%q", sc.API.DisplayTimezone)
	}
}

