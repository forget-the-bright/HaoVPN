package netutil

import "testing"

// TestSplitRemoteAddr 覆盖 IPv4、IPv6 方括号与裸地址回落。
func TestSplitRemoteAddr(t *testing.T) {
	ip, port := SplitRemoteAddr("203.0.113.1:8443")
	if ip != "203.0.113.1" || port != "8443" {
		t.Fatalf("ipv4: got %q %q", ip, port)
	}
	ip, port = SplitRemoteAddr("[2001:db8::1]:443")
	if ip != "2001:db8::1" || port != "443" {
		t.Fatalf("ipv6: got %q %q", ip, port)
	}
	ip, port = SplitRemoteAddr("10.0.0.1")
	if ip != "10.0.0.1" || port != "" {
		t.Fatalf("bare: got %q %q", ip, port)
	}
}
