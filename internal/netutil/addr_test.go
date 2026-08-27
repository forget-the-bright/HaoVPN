package netutil

import (
	"net"
	"testing"
)

func TestHostFromAddr(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"203.0.113.1:8443", "203.0.113.1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"10.0.0.1", "10.0.0.1"},
		{"localhost:8080", "localhost"},
	}
	for _, tc := range tests {
		if got := HostFromAddr(tc.in); got != tc.want {
			t.Fatalf("HostFromAddr(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseHostIP(t *testing.T) {
	ip, err := ParseHostIP("192.168.1.5:12345")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "192.168.1.5" {
		t.Fatalf("ip=%s", ip)
	}
	if _, err := ParseHostIP("not-an-ip:1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeIPv4(t *testing.T) {
	got, err := NormalizeIPv4("10.88.0.2")
	if err != nil || got != "10.88.0.2" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := NormalizeIPv4("::1"); err == nil {
		t.Fatal("expected error for non-ipv4")
	}
}

func TestDedupTrimNonEmpty(t *testing.T) {
	got := DedupTrimNonEmpty([]string{"192.168.1.0/24", "", "192.168.1.0/24", "10.0.0.0/8"})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestSplitCIDR(t *testing.T) {
	dest, mask, err := SplitCIDR("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if dest != "192.168.1.0" || mask != "255.255.255.0" {
		t.Fatalf("dest=%s mask=%s", dest, mask)
	}
	if _, _, err := SplitCIDR("2001:db8::/32"); err == nil {
		t.Fatal("expected ipv4-only error")
	}
}

func TestParseCIDRToV4Mask(t *testing.T) {
	ip, mask, err := ParseCIDRToV4Mask("192.168.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if ip != 0xc0a80100 {
		t.Fatalf("ip=%x", ip)
	}
	if mask != 0xffffff00 {
		t.Fatalf("mask=%x", mask)
	}
}

func TestParseHostIPMatchesNetParse(t *testing.T) {
	addr := "[::1]:443"
	host := HostFromAddr(addr)
	if net.ParseIP(host) == nil {
		t.Fatalf("host %q not parsed", host)
	}
}
