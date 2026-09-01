package netutil

import "testing"

func TestProbeIPForCIDR(t *testing.T) {
	if got := ProbeIPForCIDR("192.168.1.0/24"); got != "192.168.1.1" {
		t.Fatalf("got %q", got)
	}
	if got := ProbeIPForCIDR("bad"); got != "192.168.1.1" {
		t.Fatalf("fallback %q", got)
	}
}
