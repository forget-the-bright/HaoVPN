package clientapp

import (
	"net"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/netutil"
)

// TestShouldUploadTUN 源合法且目的在 AllowedIPs 才上送；公网/ICS/广播丢弃。
func TestShouldUploadTUN(t *testing.T) {
	_, lan, _ := net.ParseCIDR("192.168.3.0/24")
	_, allowedLAN, _ := net.ParseCIDR("192.168.1.0/24")
	_, vpnNet, _ := net.ParseCIDR("10.88.0.0/24")
	rt := &runtime{
		cfg:         &config.ClientConfig{},
		vpnIP:       "10.88.0.2",
		exitLANNets: []*net.IPNet{lan},
		allowedNets: []*net.IPNet{allowedLAN, vpnNet},
	}

	mk := func(src, dst string) []byte {
		pkt := make([]byte, 20)
		pkt[0] = 0x45
		copy(pkt[12:16], net.ParseIP(src).To4())
		copy(pkt[16:20], net.ParseIP(dst).To4())
		return pkt
	}

	if !rt.shouldUploadTUN(mk("10.88.0.2", "192.168.1.1")) {
		t.Fatal("VPN 源 + AllowedIPs 目的应上送")
	}
	if !rt.shouldUploadTUN(mk("192.168.3.1", "10.88.0.87")) {
		t.Fatal("local_lans 源 + VPN 子网目的应上送（via 回程）")
	}
	if rt.shouldUploadTUN(mk("10.88.0.2", "223.5.5.5")) {
		t.Fatal("公网 DNS 目的应丢弃（越权）")
	}
	if rt.shouldUploadTUN(mk("10.88.0.2", "192.168.31.1")) {
		t.Fatal("非 AllowedIPs 家宽网关应丢弃")
	}
	if rt.shouldUploadTUN(mk("192.168.3.1", "8.8.8.8")) {
		t.Fatal("LAN 访客公网浏览不得进隧道")
	}
	if rt.shouldUploadTUN(mk("192.168.137.1", "10.88.0.87")) {
		t.Fatal("ICS 源应丢弃")
	}
	if rt.shouldUploadTUN(mk("10.88.0.2", "255.255.255.255")) {
		t.Fatal("受限广播应丢弃")
	}
	if !rt.shouldUploadTUN(mk("10.88.0.2", "10.88.0.2")) {
		t.Fatal("目的为本机 VPN IP 应上送")
	}
}

// TestVPNIPOrInNetsViaNetutil 钉死与 netutil 公式一致（原 dstAllowedForUpload）。
func TestVPNIPOrInNetsViaNetutil(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.3.0/24")
	if !netutil.VPNIPOrInNets("10.88.0.2", []*net.IPNet{n}, net.ParseIP("192.168.3.10")) {
		t.Fatal("in allowed")
	}
	if netutil.VPNIPOrInNets("10.88.0.2", []*net.IPNet{n}, net.ParseIP("1.1.1.1")) {
		t.Fatal("public must deny")
	}
	if !netutil.VPNIPOrInNets("10.88.0.2", nil, net.ParseIP("10.88.0.2")) {
		t.Fatal("self vpn ip")
	}
}
