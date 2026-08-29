package clientapp

import "testing"

// TestGatewayHostRouteNeeded 已被 AllowedIPs 覆盖时不装网关 /32。
func TestGatewayHostRouteNeeded(t *testing.T) {
	if gatewayHostRouteNeeded("10.88.0.1", []string{"10.88.0.0/24"}) {
		t.Fatal("子网已覆盖网关时不应再要 /32")
	}
	if !gatewayHostRouteNeeded("10.88.0.1", []string{"192.168.3.0/24"}) {
		t.Fatal("未覆盖时应需要网关 /32")
	}
	if gatewayHostRouteNeeded("10.88.0.1", []string{"10.88.0.1/32"}) {
		t.Fatal("精确 /32 已覆盖")
	}
	if gatewayHostRouteNeeded("", nil) {
		t.Fatal("空网关")
	}
}
