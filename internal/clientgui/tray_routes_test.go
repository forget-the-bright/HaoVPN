package clientgui

import (
	"strings"
	"testing"

	"haovpn/internal/tunnel"
)

func TestTrayRouteLinesIncludesAllowedLAN(t *testing.T) {
	lines := trayRouteLines(
		"10.88.0.0/24",
		"10.88.0.2",
		"10.88.0.1",
		[]string{"10.88.0.0/24", "192.168.3.0/24"},
		nil,
	)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "10.88.0.0/24 via 10.88.0.1 (本机TUN)") {
		t.Fatalf("缺少本机TUN行: %v", lines)
	}
	if !strings.Contains(joined, "192.168.3.0/24 via 10.88.0.1") {
		t.Fatalf("分流应展示 allow_lan_cidrs: %v", lines)
	}
	// VPN 子网不应在分流区重复
	splitIdx := indexOf(lines, "—— 分流 ——")
	peerIdx := indexOf(lines, "—— 对端托管 ——")
	if splitIdx < 0 || peerIdx < 0 {
		t.Fatalf("缺少分栏: %v", lines)
	}
	for _, line := range lines[splitIdx+1 : peerIdx] {
		if strings.HasPrefix(line, "10.88.0.0/24") {
			t.Fatalf("分流区不应重复 VPN 子网: %v", lines)
		}
	}
	if !strings.Contains(joined, "（无对端托管路由）") {
		t.Fatalf("无托管时应提示: %v", lines)
	}
}

func TestFormatManagedRouteStale(t *testing.T) {
	got := formatManagedRouteLine(tunnel.ManagedRoute{Dest: "192.168.31.0/24", ViaIP: "10.88.0.5", Stale: true})
	if !strings.Contains(got, "（失效）") {
		t.Fatalf("Stale 应标失效: %q", got)
	}
	offline := formatManagedRouteLine(tunnel.ManagedRoute{Dest: "192.168.31.0/24", ViaUsername: "home", Stale: true})
	if !strings.Contains(offline, "(离线)") || !strings.Contains(offline, "（失效）") {
		t.Fatalf("离线+失效: %q", offline)
	}
}

func TestFormatTUNLinePrefersVPNSubnet(t *testing.T) {
	got := formatTUNLine("10.99.0.0/16", "10.99.0.2", "10.99.0.1")
	if got != "10.99.0.0/16 via 10.99.0.1 (本机TUN)" {
		t.Fatalf("应使用真实 vpn_subnet: %q", got)
	}
	fallback := formatTUNLine("", "10.88.0.2", "10.88.0.1")
	if fallback != "10.88.0.0/24 via 10.88.0.1 (本机TUN)" {
		t.Fatalf("缺省应 /24 回退: %q", fallback)
	}
}

func TestTrayRouteLinesManagedPeers(t *testing.T) {
	lines := trayRouteLines("10.88.0.0/24", "10.88.0.2", "10.88.0.1",
		[]string{"192.168.3.0/24"},
		[]tunnel.ManagedRoute{{Dest: "192.168.31.0/24", ViaIP: "10.88.0.9"}},
	)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "（无对端托管路由）") {
		t.Fatalf("有托管时不应空提示: %v", lines)
	}
	if !strings.Contains(joined, "192.168.31.0/24 via 10.88.0.9") {
		t.Fatalf("应列出托管: %v", lines)
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}
