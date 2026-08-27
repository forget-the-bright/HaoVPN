package netutil_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestResolveMTU(t *testing.T) {
	if got := netutil.ResolveMTU(); got != netutil.DefaultMTU {
		t.Fatalf("empty: got %d", got)
	}
	if got := netutil.ResolveMTU(0, 0, 1400); got != 1400 {
		t.Fatalf("third: got %d", got)
	}
	if got := netutil.ResolveMTU(1500, 1400); got != 1500 {
		t.Fatalf("first wins: got %d", got)
	}
}

func TestReadBufferSize(t *testing.T) {
	if got := netutil.ReadBufferSize(1400); got != 1500 {
		t.Fatalf("1400+100=%d", got)
	}
	if got := netutil.ReadBufferSize(0); got != netutil.DefaultMTU+netutil.TunReadBufferExtra {
		t.Fatalf("zero mtu: got %d", got)
	}
}
