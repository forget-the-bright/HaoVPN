package netutil_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestValidateIPOrCIDR(t *testing.T) {
	if err := netutil.ValidateIPOrCIDR("ip", "10.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	if err := netutil.ValidateIPOrCIDR("ip", "10.0.0.1", true); err != nil {
		t.Fatal(err)
	}
	if err := netutil.ValidateIPOrCIDR("ip", "203.0.113.0/24", false); err == nil {
		t.Fatal("single IP mode should reject CIDR")
	}
	if err := netutil.ValidateIPOrCIDR("ip", "203.0.113.0/24", true); err != nil {
		t.Fatalf("CIDR: %v", err)
	}
	if err := netutil.ValidateIPOrCIDR("ip", "not-an-ip", true); err == nil {
		t.Fatal("expected error for garbage")
	}
	if err := netutil.ValidateIPOrCIDR("ip", "", true); err == nil {
		t.Fatal("expected error for empty")
	}
}
