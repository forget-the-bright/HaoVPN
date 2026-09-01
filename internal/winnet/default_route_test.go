package winnet_test

import (
	"net"
	"testing"

	"haovpn/internal/winnet"
)

// TestIsIPv4DefaultRouteOnIf 纯函数：仅匹配指定 if 上的 0.0.0.0/0。
func TestIsIPv4DefaultRouteOnIf(t *testing.T) {
	if !winnet.IsIPv4DefaultRouteOnIf(23, 23, net.IPv4zero, 0) {
		t.Fatal("同 if 0.0.0.0/0 应匹配")
	}
	if winnet.IsIPv4DefaultRouteOnIf(23, 24, net.IPv4zero, 0) {
		t.Fatal("不同 if 不应匹配")
	}
	if winnet.IsIPv4DefaultRouteOnIf(23, 23, net.ParseIP("10.0.0.0"), 0) {
		t.Fatal("非默认 dest 不应匹配")
	}
	if winnet.IsIPv4DefaultRouteOnIf(23, 23, net.IPv4zero, 24) {
		t.Fatal("非 /0 前缀不应匹配")
	}
	if winnet.IsIPv4DefaultRouteOnIf(0, 23, net.IPv4zero, 0) {
		t.Fatal("ifIndex<=0 不应匹配")
	}
}
