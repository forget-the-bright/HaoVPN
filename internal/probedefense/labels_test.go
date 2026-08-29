package probedefense_test

import (
	"errors"
	"net"
	"os"
	"testing"

	"haovpn/internal/probedefense"
)

// timeoutErr 模拟带 Timeout() 的 net.Error（心跳读 deadline）。
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestIsIgnorableTransportError(t *testing.T) {
	if !probedefense.IsIgnorableTransportError(timeoutErr{}) {
		t.Fatal("Timeout() 应忽略")
	}
	if !probedefense.IsIgnorableTransportError(os.ErrDeadlineExceeded) {
		t.Fatal("deadline 应忽略")
	}
	if !probedefense.IsIgnorableTransportError(net.ErrClosed) {
		t.Fatal("closed 应忽略")
	}
	if probedefense.IsIgnorableTransportError(errors.New("tls: first record does not look like a TLS handshake")) {
		t.Fatal("真 TLS 错不应忽略")
	}
}

func TestSignatureLabelsCoverKnown(t *testing.T) {
	cases := []struct{ code, want string }{
		{"account_online", "账号已在其他设备在线"},
		{"auth_failed", "用户名或密码错误"},
		{"http_get", "HTTP GET 探测"},
	}
	for _, c := range cases {
		if got := probedefense.SignatureLabel(c.code); got != c.want {
			t.Fatalf("%s: got %q want %q", c.code, got, c.want)
		}
	}
	if got := probedefense.FormatCodeZH("auth_failed", probedefense.SignatureLabel("auth_failed")); got != "用户名或密码错误 (auth_failed)" {
		t.Fatalf("FormatCodeZH: %s", got)
	}
	if probedefense.PhaseLabel(probedefense.PhaseHandshake) != "账号握手" {
		t.Fatal("phase")
	}
	if probedefense.ActionLabel(probedefense.ActionRejected) != "已拒绝" {
		t.Fatal("action")
	}
}
