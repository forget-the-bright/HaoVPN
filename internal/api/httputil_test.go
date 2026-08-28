package api

import (
	"net/http"
	"testing"
)

// TestResolveClientIP 验证默认仅用 RemoteAddr；信任反代时才解析 XFF。
func TestResolveClientIP(t *testing.T) {
	r := &http.Request{Header: http.Header{}}
	r.RemoteAddr = "203.0.113.10:54321"
	if got := resolveClientIP(r, nil); got != "203.0.113.10" {
		t.Fatalf("IPv4 RemoteAddr: got %q", got)
	}

	r.RemoteAddr = "[2001:db8::1]:443"
	if got := resolveClientIP(r, nil); got != "2001:db8::1" {
		t.Fatalf("IPv6 RemoteAddr: got %q", got)
	}

	r.RemoteAddr = "203.0.113.10:54321"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := resolveClientIP(r, nil); got != "203.0.113.10" {
		t.Fatalf("无 trusted proxy 应忽略 XFF: got %q", got)
	}

	r.RemoteAddr = "127.0.0.1:12345"
	r.Header.Set("X-Forwarded-For", " 198.51.100.1 , 203.0.113.1")
	if got := resolveClientIP(r, []string{"127.0.0.1/32"}); got != "198.51.100.1" {
		t.Fatalf("trusted proxy XFF: got %q", got)
	}

	r.RemoteAddr = "10.0.0.1:9999"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	if got := resolveClientIP(r, nil); got != "10.0.0.1" {
		t.Fatalf("锁定 IP 应固定为 RemoteAddr: got %q", got)
	}
	r.Header.Set("X-Forwarded-For", "2.2.2.2")
	if got := resolveClientIP(r, nil); got != "10.0.0.1" {
		t.Fatalf("轮换 XFF 不应改变锁定 IP: got %q", got)
	}
}
