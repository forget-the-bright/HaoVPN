package tunnel_test

import (
	"strings"
	"testing"

	"haovpn/internal/tunnel"
)

func TestCheckTunnelSourceIP(t *testing.T) {
	allowed := []string{"10.0.0.0/8", "192.168.1.50"}
	if err := tunnel.CheckTunnelSourceIP("10.1.2.3:8443", allowed); err != nil {
		t.Fatalf("10.1.2.3 should be allowed: %v", err)
	}
	if err := tunnel.CheckTunnelSourceIP("8.8.8.8:8443", allowed); err == nil {
		t.Fatal("8.8.8.8 should be denied")
	}
	if err := tunnel.CheckTunnelSourceIP("192.168.1.50:1234", allowed); err != nil {
		t.Fatalf("single IP rule: %v", err)
	}
	if err := tunnel.CheckTunnelSourceIP("1.2.3.4:8443", nil); err != nil {
		t.Fatal("empty allow list should permit all")
	}
	if err := tunnel.CheckTunnelSourceIP("bad", allowed); err == nil || !strings.Contains(err.Error(), "解析") {
		t.Fatalf("bad addr: %v", err)
	}
}
