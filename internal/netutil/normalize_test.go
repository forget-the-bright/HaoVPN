package netutil_test

import (
	"net"
	"testing"

	"haovpn/internal/netutil"
)

// TestNormalizeCIDROrHost 单 IP→/32；CIDR 规范化。
func TestNormalizeCIDROrHost(t *testing.T) {
	s, err := netutil.NormalizeCIDROrHost("10.88.0.5")
	if err != nil || s != "10.88.0.5/32" {
		t.Fatalf("got %q err=%v", s, err)
	}
	s, err = netutil.NormalizeCIDROrHost("192.168.1.0/24")
	if err != nil || s != "192.168.1.0/24" {
		t.Fatalf("got %q err=%v", s, err)
	}
}

// TestForbidDefaultRoute 拒绝 /0。
func TestForbidDefaultRoute(t *testing.T) {
	if err := netutil.ForbidDefaultRoute("0.0.0.0/0"); err == nil {
		t.Fatal("应拒绝默认路由")
	}
	if err := netutil.ForbidDefaultRoute("10.0.0.0/8"); err != nil {
		t.Fatal(err)
	}
}

// TestValidateAdvertisedLAN RFC1918 + ≥/16。
func TestValidateAdvertisedLAN(t *testing.T) {
	s, err := netutil.ValidateAdvertisedLAN("192.168.3.0/24")
	if err != nil || s != "192.168.3.0/24" {
		t.Fatalf("got %q err=%v", s, err)
	}
	if _, err := netutil.ValidateAdvertisedLAN("10.0.0.0/8"); err == nil {
		t.Fatal("应拒绝过宽 /8")
	}
	if _, err := netutil.ValidateAdvertisedLAN("8.8.8.0/24"); err == nil {
		t.Fatal("应拒绝公网")
	}
	if _, err := netutil.ValidateAdvertisedLAN("0.0.0.0/0"); err == nil {
		t.Fatal("应拒绝默认路由")
	}
	s, err = netutil.ValidateAdvertisedLAN("10.1.2.3")
	if err != nil || s != "10.1.2.3/32" {
		t.Fatalf("单 IP got %q err=%v", s, err)
	}
}

// TestIsLimitedBroadcast 255.255.255.255。
func TestIsLimitedBroadcast(t *testing.T) {
	if !netutil.IsLimitedBroadcast(net.IPv4(255, 255, 255, 255)) {
		t.Fatal("expected true")
	}
	if netutil.IsLimitedBroadcast(net.IPv4(10, 0, 0, 1)) {
		t.Fatal("expected false")
	}
}

// TestNormalizeRemoteHost loopback 归一与 IPv4。
func TestNormalizeRemoteHost(t *testing.T) {
	if got := netutil.NormalizeRemoteHost("[::1]:8443"); got != "127.0.0.1" {
		t.Fatalf("got %q", got)
	}
	if got := netutil.NormalizeRemoteHost("10.88.0.3:443"); got != "10.88.0.3" {
		t.Fatalf("got %q", got)
	}
}

// TestValidLANCIDRs 过滤无效项并保序去重。
func TestValidLANCIDRs(t *testing.T) {
	if got := netutil.ValidLANCIDRs(nil); len(got) != 0 {
		t.Fatalf("nil -> %v", got)
	}
	got := netutil.ValidLANCIDRs([]string{"192.168.1.0/24", "", "0.0.0.0/0", "192.168.1.0/24", "8.8.8.0/24"})
	if len(got) != 1 || got[0] != "192.168.1.0/24" {
		t.Fatalf("got %v", got)
	}
}

// TestNormalizeCIDRList 规范化、去重、排序。
func TestNormalizeCIDRList(t *testing.T) {
	got := netutil.NormalizeCIDRList([]string{"10.88.0.5", "10.88.0.0/24", "10.88.0.5/32", "bad"}, netutil.NormalizeCIDRListOpts{Sort: true})
	if len(got) != 2 || got[0] != "10.88.0.0/24" || got[1] != "10.88.0.5/32" {
		t.Fatalf("got %v", got)
	}
}

// TestAppendCIDRUnique skipIfCovered 去冗余 /32。
func TestAppendCIDRUnique(t *testing.T) {
	var cidrs []string
	var nets []*net.IPNet
	cidrs, nets, ok := netutil.AppendCIDRUnique(cidrs, nets, "10.88.0.0/24", false)
	if !ok || len(cidrs) != 1 {
		t.Fatalf("base %v ok=%v", cidrs, ok)
	}
	cidrs, nets, ok = netutil.AppendCIDRUnique(cidrs, nets, "10.88.0.5/32", true)
	if ok {
		t.Fatalf("应跳过被覆盖的 /32, got %v nets=%v", cidrs, nets)
	}
	cidrs, nets, ok = netutil.AppendCIDRUnique(cidrs, nets, "10.88.1.1/32", true)
	if !ok || len(cidrs) != 2 {
		t.Fatalf("未覆盖 /32 应追加, got %v ok=%v", cidrs, ok)
	}
}

