package netstack_test

import (
	"testing"

	"haovpn/internal/netstack"
)

// TestProbeIPForCIDR 验证 LAN 探测 IP 生成。
func TestProbeIPForCIDR(t *testing.T) {
	if got := netstack.ProbeIPForCIDR("192.168.1.0/24"); got != "192.168.1.1" {
		t.Fatalf("probe=%s", got)
	}
}

// TestSplitCIDR 验证 Windows route 所需的 dest/mask 拆分。
func TestSplitCIDR(t *testing.T) {
	dest, mask, err := netstack.SplitCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if dest != "192.168.1.0" {
		t.Fatalf("dest=%s", dest)
	}
	if mask != "255.255.255.0" {
		t.Fatalf("mask=%s", mask)
	}
}

// TestWindowsOnLinkRouteArgs 确认 Wintun 路由用 0.0.0.0 + IF，避免 /32 下一跳不可达。
func TestWindowsOnLinkRouteArgs(t *testing.T) {
	args := netstack.WindowsOnLinkRouteArgs("10.88.0.0", "255.255.255.0", 42)
	want := []string{"ADD", "10.88.0.0", "MASK", "255.255.255.0", "0.0.0.0", "IF", "42"}
	if len(args) != len(want) {
		t.Fatalf("args=%v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args=%v want=%v", args, want)
		}
	}
}

// TestParseCIDRs 边界：非法 CIDR 必须失败。
func TestParseCIDRs(t *testing.T) {
	if _, err := netstack.ParseCIDRs([]string{"10.0.0.0/24", "bad"}); err == nil {
		t.Fatal("非法 CIDR 应失败")
	}
}
