package transport_test

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// TestCloseInvokesOnClose Close 须在锁保护下拷贝并调用 onClose（防 SetOnClose 竞态）。
func TestCloseInvokesOnClose(t *testing.T) {
	cert := testTLSCert(t)
	cfg := transport.DefaultConfig()
	cfg.HeartbeatInterval = time.Hour
	cfg.HeartbeatTimeout = time.Hour

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan *transport.Conn, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tlsConn := tls.Server(raw, security.TLSConfig(cert, true))
		if err := tlsConn.Handshake(); err != nil {
			_ = raw.Close()
			return
		}
		accepted <- transport.AcceptConn(tlsConn, cfg, nil, nil)
	}()

	cliTLS := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	c, err := transport.Dial(ln.Addr().String(), cliTLS, cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	sc := <-accepted

	var calls atomic.Int32
	sc.SetOnClose(func(error) { calls.Add(1) })
	if err := sc.Close(); err != nil {
		t.Logf("close err (可忽略): %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("onClose 应调用 1 次, got %d", calls.Load())
	}
	// 重复 Close 不得再调
	_ = sc.Close()
	if calls.Load() != 1 {
		t.Fatalf("重复 Close 不应再调 onClose, got %d", calls.Load())
	}
}
