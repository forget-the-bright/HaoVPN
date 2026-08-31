package netutil_test

import (
	"errors"
	"testing"

	"haovpn/internal/dialerr"
	"haovpn/internal/netutil"
)

func TestCheckSourceIPAllowedWrapsSentinel(t *testing.T) {
	allowed := []string{"10.0.0.0/8"}
	if err := netutil.CheckSourceIPAllowed("10.1.2.3:9", allowed); err != nil {
		t.Fatalf("allow: %v", err)
	}
	err := netutil.CheckSourceIPAllowed("8.8.8.8:9", allowed)
	if err == nil {
		t.Fatal("expected deny")
	}
	if !errors.Is(err, dialerr.ErrSourceDenied) {
		t.Fatalf("want dialerr.ErrSourceDenied, got %v", err)
	}
	if err := netutil.CheckSourceIPAllowed("1.2.3.4", nil); err != nil {
		t.Fatal("empty list permits all")
	}
}

// TestCheckSourceIPAllowedRules 覆盖 CIDR/单 IP/空列表/坏地址（原 tunnel.CheckTunnelSourceIP 用例迁入）。
func TestCheckSourceIPAllowedRules(t *testing.T) {
	allowed := []string{"10.0.0.0/8", "192.168.1.50"}
	if err := netutil.CheckSourceIPAllowed("10.1.2.3:8443", allowed); err != nil {
		t.Fatalf("10.1.2.3 should be allowed: %v", err)
	}
	if err := netutil.CheckSourceIPAllowed("8.8.8.8:8443", allowed); err == nil {
		t.Fatal("8.8.8.8 should be denied")
	} else if !errors.Is(err, dialerr.ErrSourceDenied) {
		t.Fatalf("deny should wrap ErrSourceDenied: %v", err)
	}
	if err := netutil.CheckSourceIPAllowed("192.168.1.50:1234", allowed); err != nil {
		t.Fatalf("single IP rule: %v", err)
	}
	if err := netutil.CheckSourceIPAllowed("1.2.3.4:8443", nil); err != nil {
		t.Fatal("empty allow list should permit all")
	}
	if err := netutil.CheckSourceIPAllowed("bad", allowed); err == nil {
		t.Fatal("bad addr should error")
	}
}
