package netutil_test

import (
	"testing"

	"haovpn/internal/netutil"
)

func TestInferGatewayFromVPNIP(t *testing.T) {
	if got := netutil.InferGatewayFromVPNIP("10.88.0.50"); got != "10.88.0.1" {
		t.Fatalf("got %s", got)
	}
	if got := netutil.InferGatewayFromVPNIP("bad"); got != "10.88.0.1" {
		t.Fatalf("fallback got %s", got)
	}
}

func TestInferVPNSubnetHint(t *testing.T) {
	if got := netutil.InferVPNSubnetHint("10.88.0.50"); got != "10.88.0.0/24" {
		t.Fatalf("got %s", got)
	}
	if got := netutil.InferVPNSubnetHint("  10.88.0.5  "); got != "10.88.0.0/24" {
		t.Fatalf("trim got %s", got)
	}
	if got := netutil.InferVPNSubnetHint("bad"); got != "bad" {
		t.Fatalf("invalid fallback got %q", got)
	}
	if got := netutil.InferVPNSubnetHint("  "); got != "" {
		t.Fatalf("blank got %q", got)
	}
}

func TestTrimLower(t *testing.T) {
	if got := netutil.TrimLower("  FooBar  "); got != "foobar" {
		t.Fatalf("got %q", got)
	}
	if got := netutil.TrimLower(""); got != "" {
		t.Fatalf("empty got %q", got)
	}
}

func TestResolveGatewayPriority(t *testing.T) {
	if got := netutil.ResolveGateway("10.1.0.1", "10.88.0.5"); got != "10.1.0.1" {
		t.Fatalf("handshake wins: %s", got)
	}
	if got := netutil.ResolveGateway("", "10.88.0.5"); got != "10.88.0.1" {
		t.Fatalf("infer: %s", got)
	}
	if got := netutil.ResolveGateway("  ", "10.88.0.5"); got != "10.88.0.1" {
		t.Fatalf("blank handshake → infer: %s", got)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	if !netutil.IsLoopbackHost("127.0.0.1") {
		t.Fatal("127.0.0.1")
	}
	if netutil.IsLoopbackHost("192.168.1.1") {
		t.Fatal("not loopback")
	}
}
