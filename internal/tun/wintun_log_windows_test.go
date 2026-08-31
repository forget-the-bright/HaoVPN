//go:build windows

package tun

import (
	"strings"
	"testing"

	"haovpn/internal/brand"
	"haovpn/internal/winnet"
)

func TestIsExpectedWintunDebug(t *testing.T) {
	cases := []string{
		"Failed to find matching adapter name: 找不到元素。 (Code 0x00000490)",
		"Creating adapter",
		"Using existing driver 0.14",
		`Removed orphaned adapter "haovpn0 1"`,
	}
	for _, msg := range cases {
		if !isExpectedWintunDebug(msg) {
			t.Errorf("expected debug for %q", msg)
		}
	}
	if isExpectedWintunDebug("driver install failed") {
		t.Error("unexpected debug for real failure")
	}
}

func TestBuildPrepareWintunPSScript(t *testing.T) {
	ps := winnet.BuildPrepareWintunOrphanScript("haovpn0")
	for _, frag := range []string{"haovpn0", "Remove-NetAdapter", brand.WintunPool} {
		if !strings.Contains(ps, frag) {
			t.Fatalf("script missing %q: %s", frag, ps)
		}
	}
}
