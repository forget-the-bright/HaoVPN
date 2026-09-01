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

// TestIPInAnyNetAndVPNIPOrInNets 热路径网段命中与 VPN IP 短路。
func TestIPInAnyNetAndVPNIPOrInNets(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.3.0/24")
	nets := []*net.IPNet{n, nil}
	if !netutil.IPInAnyNet(nets, net.ParseIP("192.168.3.10")) {
		t.Fatal("in net")
	}
	if netutil.IPInAnyNet(nets, net.ParseIP("10.0.0.1")) {
		t.Fatal("out of net")
	}
	if !netutil.VPNIPOrInNets("10.88.0.2", nets, net.ParseIP("10.88.0.2")) {
		t.Fatal("self vpn")
	}
	if !netutil.VPNIPOrInNets("10.88.0.2", nets, net.ParseIP("192.168.3.1")) {
		t.Fatal("in allowed")
	}
	if netutil.VPNIPOrInNets("10.88.0.2", nets, net.ParseIP("1.1.1.1")) {
		t.Fatal("public deny")
	}
	if netutil.VPNIPOrInNets("10.88.0.2", nil, nil) {
		t.Fatal("nil ip")
	}
}

// TestIsTUNNoiseDstVsForLog 客户端噪声不含 LL-unicast；服务端日志噪声含。
func TestIsTUNNoiseDstVsForLog(t *testing.T) {
	if !netutil.IsTUNNoiseDst(net.IPv4(224, 0, 0, 251)) {
		t.Fatal("mcast")
	}
	if !netutil.IsTUNNoiseDst(net.IPv4(255, 255, 255, 255)) {
		t.Fatal("limited broadcast")
	}
	ll := net.IPv4(169, 254, 1, 1)
	if netutil.IsTUNNoiseDst(ll) {
		t.Fatal("LL-unicast must NOT be client noise")
	}
	if !netutil.IsTUNNoiseForLog(ll) {
		t.Fatal("LL-unicast is server log noise")
	}
}

// TestIsTUNNoiseSource 未指定 / LL / nil 为源噪声；普通单播不是。
func TestIsTUNNoiseSource(t *testing.T) {
	if !netutil.IsTUNNoiseSource(nil) {
		t.Fatal("nil")
	}
	if !netutil.IsTUNNoiseSource(net.IPv4(0, 0, 0, 0)) {
		t.Fatal("unspecified")
	}
	if !netutil.IsTUNNoiseSource(net.IPv4(169, 254, 1, 1)) {
		t.Fatal("LL-unicast")
	}
	if netutil.IsTUNNoiseSource(net.IPv4(10, 88, 0, 5)) {
		t.Fatal("VPN IP must not be source noise")
	}
}

// TestMergeDNSIntoAllowedIPs 公网 DNS 追加 /32；已被 VPN 子网覆盖的 gateway 不重复。
func TestMergeDNSIntoAllowedIPs(t *testing.T) {
	got := netutil.MergeDNSIntoAllowedIPs(
		[]string{"10.88.0.0/24", "192.168.3.0/24"},
		[]string{"223.5.5.5", "10.88.0.1", "  "},
	)
	hasDNS, hasGWDup := false, false
	for _, c := range got {
		if c == "223.5.5.5/32" {
			hasDNS = true
		}
		if c == "10.88.0.1/32" {
			hasGWDup = true
		}
	}
	if !hasDNS {
		t.Fatalf("missing public DNS /32: %v", got)
	}
	if hasGWDup {
		t.Fatalf("gateway already in /24 must not add /32: %v", got)
	}
	if len(netutil.MergeDNSIntoAllowedIPs([]string{"10.0.0.0/8"}, nil)) != 1 {
		t.Fatal("empty dns keeps allowed")
	}
}

