package clientapp

import (
	"net"
	"testing"

	"haovpn/internal/config"
)

// TestShouldUploadTUN 仅 VPN IP 与 local_lans 源可上送；ICS/广播丢弃。
func TestShouldUploadTUN(t *testing.T) {
	_, lan, _ := net.ParseCIDR("192.168.3.0/24")
	rt := &runtime{
		cfg:         &config.ClientConfig{},
		vpnIP:       "10.88.0.2",
		exitLANNets: []*net.IPNet{lan},
	}

	mk := func(src, dst string) []byte {
		pkt := make([]byte, 20)
		pkt[0] = 0x45
		copy(pkt[12:16], net.ParseIP(src).To4())
		copy(pkt[16:20], net.ParseIP(dst).To4())
		return pkt
	}

	if !rt.shouldUploadTUN(mk("10.88.0.2", "192.168.1.1")) {
		t.Fatal("VPN 源应上送")
	}
	if !rt.shouldUploadTUN(mk("192.168.3.1", "10.88.0.87")) {
		t.Fatal("local_lans 源应上送（via 回程）")
	}
	if rt.shouldUploadTUN(mk("192.168.137.1", "10.88.0.87")) {
		t.Fatal("ICS 源应丢弃")
	}
	if rt.shouldUploadTUN(mk("10.88.0.2", "255.255.255.255")) {
		t.Fatal("受限广播应丢弃")
	}
}
