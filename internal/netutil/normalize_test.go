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

// TestValidateAdvertisedLANNotForbidden 拒绝与 VPN 池重叠，放行不重叠私网。
func TestValidateAdvertisedLANNotForbidden(t *testing.T) {
	vpn := "10.88.0.0/24"
	if _, err := netutil.ValidateAdvertisedLANNotForbidden("10.88.0.0/24", vpn); err == nil {
		t.Fatal("应拒绝与 VPN 池完全相同")
	}
	if _, err := netutil.ValidateAdvertisedLANNotForbidden("10.88.0.5/32", vpn); err == nil {
		t.Fatal("应拒绝落在 VPN 池内的 /32")
	}
	if _, err := netutil.ValidateAdvertisedLANNotForbidden("10.88.0.0/16", vpn); err == nil {
		t.Fatal("应拒绝覆盖 VPN 池的更宽前缀")
	}
	s, err := netutil.ValidateAdvertisedLANNotForbidden("192.168.31.0/24", vpn)
	if err != nil || s != "192.168.31.0/24" {
		t.Fatalf("不重叠应通过 got %q err=%v", s, err)
	}
	got := netutil.ValidLANCIDRsNotForbidden(
		[]string{"192.168.1.0/24", "10.88.0.0/24", "10.0.0.0/8"}, vpn)
	if len(got) != 1 || got[0] != "192.168.1.0/24" {
		t.Fatalf("过滤后应仅私网 LAN, got %v", got)
	}
}

// TestCIDRsOverlap 相邻等长不重叠；包含关系重叠。
func TestCIDRsOverlap(t *testing.T) {
	ov, err := netutil.CIDRsOverlap("10.0.0.0/25", "10.0.0.128/25")
	if err != nil || ov {
		t.Fatalf("相邻等长应不重叠 ov=%v err=%v", ov, err)
	}
	ov, err = netutil.CIDRsOverlap("10.0.0.0/24", "10.0.0.128/25")
	if err != nil || !ov {
		t.Fatalf("包含应重叠 ov=%v err=%v", ov, err)
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

// TestValidateLocalLANsList 硬校验：非法挡过；合法规范化去重。
func TestValidateLocalLANsList(t *testing.T) {
	got, err := netutil.ValidateLocalLANsList(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty: got=%v err=%v", got, err)
	}
	got, err = netutil.ValidateLocalLANsList([]string{"", "  "})
	if err != nil || len(got) != 0 {
		t.Fatalf("blank: got=%v err=%v", got, err)
	}
	got, err = netutil.ValidateLocalLANsList([]string{"192.168.31.0/24", "192.168.31.0/24"})
	if err != nil || len(got) != 1 || got[0] != "192.168.31.0/24" {
		t.Fatalf("dedup: got=%v err=%v", got, err)
	}
	_, err = netutil.ValidateLocalLANsList([]string{"192.168.1.0/24", "not-a-cidr"})
	if err == nil {
		t.Fatal("非法应失败")
	}
	_, err = netutil.ValidateLocalLANsList([]string{"0.0.0.0/0"})
	if err == nil {
		t.Fatal("默认路由应失败")
	}
	_, err = netutil.ValidateLocalLANsList([]string{"8.8.8.0/24"})
	if err == nil {
		t.Fatal("公网应失败")
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

