package probedefense_test

import (
	"testing"

	"haovpn/internal/probedefense"
)

func TestClassifyFrameLength(t *testing.T) {
	if got := probedefense.ClassifyFrameLength(1195725856); got != "http_get" {
		t.Fatalf("GET: got %s", got)
	}
	if got := probedefense.ClassifyFrameLength(1095586128); got != "amqp" {
		t.Fatalf("AMQP: got %s", got)
	}
	if got := probedefense.ClassifyFrameLength(369295360); got != "nested_tls" {
		t.Fatalf("nested tls: got %s", got)
	}
}

func TestClassifyTLSError(t *testing.T) {
	if probedefense.ClassifyTLSError(errStr("tls: unsupported SSLv2 handshake received")) != "sslv2" {
		t.Fatal("sslv2")
	}
	if probedefense.ClassifyTLSError(errStr("read: connection reset by peer")) != "connection_reset" {
		t.Fatal("rst")
	}
}

func TestOnTransportReadErrorSkipsTimeout(t *testing.T) {
	// 无 store 的 Guard：超时路径须在写库前 return（Enabled 下仍安全）
	g := probedefense.New(nil, probedefense.DefaultConfig())
	g.OnTransportReadError("1.2.3.4:9", timeoutErr{})
	// 不 panic、不写库即通过
}

type errStr string

func (e errStr) Error() string { return string(e) }
