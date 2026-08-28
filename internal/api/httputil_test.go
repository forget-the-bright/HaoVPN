package api

import (
	"net/http"
	"testing"
)

// TestClientIP 验证 XFF 优先与 IPv6 RemoteAddr 经 HostFromAddr 正确剥离端口。
func TestClientIP(t *testing.T) {
	r := &http.Request{Header: http.Header{}}
	r.RemoteAddr = "203.0.113.10:54321"
	if got := clientIP(r); got != "203.0.113.10" {
		t.Fatalf("IPv4 RemoteAddr: got %q", got)
	}

	r.RemoteAddr = "[2001:db8::1]:443"
	if got := clientIP(r); got != "2001:db8::1" {
		t.Fatalf("IPv6 RemoteAddr: got %q", got)
	}

	r.Header.Set("X-Forwarded-For", " 198.51.100.1 , 203.0.113.1")
	if got := clientIP(r); got != "198.51.100.1" {
		t.Fatalf("XFF first hop: got %q", got)
	}
}
