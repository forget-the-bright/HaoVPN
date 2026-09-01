package winnet_test

import (
	"testing"

	"haovpn/internal/brand"
	"haovpn/internal/winnet"
)

// TestIsWintunOrphanAdapterName 钉死孤儿名规则（与 PS 清理脚本语义对齐）。
func TestIsWintunOrphanAdapterName(t *testing.T) {
	cases := []struct {
		want, name, desc string
		hit              bool
	}{
		{"haovpn_client", "haovpn_client", "Wintun Tunnel", false},
		{"haovpn_client", "haovpn_client 1", "Wintun Tunnel", true},
		{"haovpn_client", "haovpn_client#2", brand.WintunPool, true},
		{"haovpn0", "haovpn0extra", "Wintun", false},
		{"haovpn0", "WLAN", "Intel", false},
		{"haovpn0", "haovpn0 1", "", true}, // 无描述仍按名
		{"", "haovpn0 1", "Wintun", false},
	}
	for _, tc := range cases {
		got := winnet.IsWintunOrphanAdapterName(tc.want, tc.name, tc.desc)
		if got != tc.hit {
			t.Fatalf("want=%q name=%q desc=%q got=%v wantHit=%v", tc.want, tc.name, tc.desc, got, tc.hit)
		}
	}
}
