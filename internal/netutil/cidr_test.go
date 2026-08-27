package netutil_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestGatewayCIDR(t *testing.T) {
	got := netutil.GatewayCIDR("10.88.0.1", "10.88.0.0/24")
	if got != "10.88.0.1/24" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateNoFullTunnel(t *testing.T) {
	if err := netutil.ValidateNoFullTunnel([]string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
	if err := netutil.ValidateNoFullTunnel([]string{"0.0.0.0/0"}); err == nil {
		t.Fatal("应拒绝全隧道")
	}
}

func TestValidateSubnetGateway(t *testing.T) {
	if err := netutil.ValidateSubnetGateway("10.88.0.0/24", "10.88.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := netutil.ValidateSubnetGateway("10.88.0.0/24", "10.99.0.1"); err == nil {
		t.Fatal("网关不在子网应失败")
	}
}
