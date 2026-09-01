package clientapp

import (
	"reflect"
	"testing"

	"haovpn/internal/netstack"
)

// TestNormalizeRouteCIDR 单 IP 与 CIDR 规范化。
func TestNormalizeRouteCIDR(t *testing.T) {
	if got := normalizeRouteCIDR("10.88.0.5"); got != "10.88.0.5/32" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeRouteCIDR("192.168.1.0/24"); got != "192.168.1.0/24" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeRouteCIDR("bad"); got != "" {
		t.Fatalf("无效应空, got %q", got)
	}
}

// TestDesiredClientRoutes 网关覆盖与 AllowedIPs 合并。
func TestDesiredClientRoutes(t *testing.T) {
	got := desiredClientRoutes("10.88.0.1", []string{"10.88.0.0/24", "192.168.3.0/24"})
	want := []string{"10.88.0.0/24", "192.168.3.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("已覆盖网关时 got=%v want=%v", got, want)
	}
	got = desiredClientRoutes("10.88.0.1", []string{"192.168.3.0/24"})
	want = []string{"10.88.0.1/32", "192.168.3.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("未覆盖网关时 got=%v want=%v", got, want)
	}
}

// TestRouteSetDiff 增删与不变。
func TestRouteSetDiff(t *testing.T) {
	add, del := routeSetDiff(
		[]string{"10.0.0.0/24", "192.168.1.0/24"},
		[]string{"10.0.0.0/24", "172.16.0.0/16"},
	)
	if !reflect.DeepEqual(add, []string{"172.16.0.0/16"}) {
		t.Fatalf("add=%v", add)
	}
	if !reflect.DeepEqual(del, []string{"192.168.1.0/24"}) {
		t.Fatalf("del=%v", del)
	}
	add, del = routeSetDiff(
		[]string{"10.0.0.0/24"},
		[]string{"10.0.0.0/24"},
	)
	if len(add) != 0 || len(del) != 0 {
		t.Fatalf("不变应空 add=%v del=%v", add, del)
	}
	// 规范化：未带 /32 的主机与带 /32 视为同一条
	add, del = routeSetDiff([]string{"10.88.0.5"}, []string{"10.88.0.5/32"})
	if len(add) != 0 || len(del) != 0 {
		t.Fatalf("规范化后应相等 add=%v del=%v", add, del)
	}
}

// TestViaFingerprint 相同配置指纹一致；空 lans 为空串；tunIP 不参与指纹。
func TestViaFingerprint(t *testing.T) {
	if viaFingerprint(nil, "10.88.0.0/24", "10.88.0.2") != "" {
		t.Fatal("空 lans 指纹应空")
	}
	a := viaFingerprint([]string{"192.168.31.0/24"}, "10.88.0.0/24", "10.88.0.2")
	b := viaFingerprint([]string{"192.168.31.0/24"}, "10.88.0.0/24", "10.88.0.2")
	if a == "" || a != b {
		t.Fatalf("相同配置指纹应一致 a=%q b=%q", a, b)
	}
	sameLansDiffIP := viaFingerprint([]string{"192.168.31.0/24"}, "10.88.0.0/24", "10.88.0.9")
	if a != sameLansDiffIP {
		t.Fatalf("tunIP 不应影响指纹 a=%q ip9=%q", a, sameLansDiffIP)
	}
	c := viaFingerprint([]string{"192.168.32.0/24"}, "10.88.0.0/24", "10.88.0.2")
	if a == c {
		t.Fatal("不同 lans 指纹应不同")
	}
	d := viaFingerprint([]string{"192.168.31.0/24"}, "10.88.0.0/16", "10.88.0.2")
	if a == d {
		t.Fatal("不同 subnet 指纹应不同")
	}
}

// TestDNSServersEqual DNS 列表比较。
func TestDNSServersEqual(t *testing.T) {
	if !dnsServersEqual([]string{"1.1.1.1", "8.8.8.8"}, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Fatal("应相等")
	}
	if dnsServersEqual([]string{"1.1.1.1"}, []string{"8.8.8.8"}) {
		t.Fatal("应不等")
	}
}

// TestRouteListsEqual 软换 IP 路由 noop 决策。
func TestRouteListsEqual(t *testing.T) {
	a := []string{"10.88.0.0/24", "192.168.3.0/24"}
	b := []string{"192.168.3.0/24", "10.88.0.0/24"}
	if !routeListsEqual(a, b) {
		t.Fatal("顺序不同应相等")
	}
	if routeListsEqual(a, []string{"10.88.0.0/24"}) {
		t.Fatal("条数不同应不等")
	}
}

func TestWillViaSetupLocked(t *testing.T) {
	rt := &runtime{}
	lans := []string{"192.168.31.0/24"}
	if !rt.willViaSetupLocked("10.88.0.0/24", "10.88.0.2", lans) {
		t.Fatal("首次有 lans 应 willViaSetup")
	}
	if rt.willViaSetupLocked("10.88.0.0/24", "10.88.0.2", nil) {
		t.Fatal("空 lans 不应 Setup")
	}
	rt.viaFP = viaFingerprint(lans, "10.88.0.0/24", "10.88.0.2")
	rt.viaFPKnown = true
	rt.via = &viaExit{stack: &netstack.Stack{}} // 非 nil 即表示已有 stack（不调用方法）
	if rt.willViaSetupLocked("10.88.0.0/24", "10.88.0.2", lans) {
		t.Fatal("指纹未变且 stack 在，不应再 Setup")
	}
	if rt.willViaSetupLocked("10.88.0.0/24", "10.88.0.9", lans) {
		t.Fatal("仅 tunIP 变不应再 Setup（指纹不含 IP）")
	}
	if !rt.willViaSetupLocked("10.88.0.0/16", "10.88.0.2", lans) {
		t.Fatal("subnet 变应再 Setup")
	}
}

