package tunnel_test

import (
	"testing"

	"haovpn/internal/tunnel"
)

// TestHandshakeRoundTrip 验证握手 JSON 编解码。
func TestHandshakeRoundTrip(t *testing.T) {
	req, err := tunnel.EncodeHandshakeRequest("test-pub-key")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := tunnel.ParseHandshakeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PublicKey != "test-pub-key" {
		t.Fatal("public key mismatch")
	}
	ok, err := tunnel.EncodeHandshakeOK("server-pub", tunnel.HandshakePolicy{
		VPNIP: "10.88.0.5", AllowedIPs: []string{"192.168.1.0/24"}, MTU: 1420, PolicyVer: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tunnel.ParseHandshakeResponse(ok)
	if err != nil || resp.ServerPublicKey != "server-pub" || resp.Policy == nil || resp.Policy.VPNIP != "10.88.0.5" {
		t.Fatal("response mismatch")
	}
}
