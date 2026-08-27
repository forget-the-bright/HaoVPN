package netutil_test

import (
	"net"
	"testing"

	"haovpn/internal/netutil"
)

func TestIPMatchesRulesCIDR(t *testing.T) {
	ip := net.ParseIP("192.168.1.10")
	if !netutil.IPMatchesRules(ip, []string{"192.168.1.0/24"}) {
		t.Fatal("should match cidr")
	}
	if netutil.IPMatchesRules(ip, []string{"10.0.0.0/8"}) {
		t.Fatal("should not match")
	}
}

func TestIPMatchesRulesSingleIP(t *testing.T) {
	ip := net.ParseIP("10.88.0.5")
	if !netutil.IPMatchesRules(ip, []string{"10.88.0.5"}) {
		t.Fatal("single ip match")
	}
}

func TestParseCIDRListToNetsEmpty(t *testing.T) {
	if _, err := netutil.ParseCIDRListToNets(nil); err == nil {
		t.Fatal("empty should error")
	}
}

func TestParseCIDRListToNetsMixed(t *testing.T) {
	nets, err := netutil.ParseCIDRListToNets([]string{"192.168.1.0/24", "bad", "10.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("len=%d", len(nets))
	}
}
