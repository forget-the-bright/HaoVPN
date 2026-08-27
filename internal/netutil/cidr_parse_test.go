package netutil

import (
	"net"
	"testing"
)

func TestParseCIDR(t *testing.T) {
	ip, n, err := ParseCIDR("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if ip == nil || n == nil {
		t.Fatal("nil result")
	}
	if _, _, err := ParseCIDR("bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestIPNetsToStrings(t *testing.T) {
	_, n, _ := net.ParseCIDR("10.0.0.0/8")
	got := IPNetsToStrings([]*net.IPNet{n, nil})
	if len(got) != 1 || got[0] != "10.0.0.0/8" {
		t.Fatalf("got %v", got)
	}
}
