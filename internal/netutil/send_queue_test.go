package netutil_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestClampSendQueueSize(t *testing.T) {
	n, ch := netutil.ClampSendQueueSize(0)
	if n != netutil.DefaultSendQueueSize || ch {
		t.Fatalf("0 → default got %d changed=%v", n, ch)
	}
	n, ch = netutil.ClampSendQueueSize(10)
	if n != netutil.MinSendQueueSize || !ch {
		t.Fatalf("too small got %d changed=%v", n, ch)
	}
	n, ch = netutil.ClampSendQueueSize(99999)
	if n != netutil.MaxSendQueueSize || !ch {
		t.Fatalf("too large got %d changed=%v", n, ch)
	}
	n, ch = netutil.ClampSendQueueSize(1024)
	if n != 1024 || ch {
		t.Fatalf("1024 got %d changed=%v", n, ch)
	}
}
