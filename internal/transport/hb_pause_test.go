package transport_test

import (
	"crypto/tls"
	"net"
	"testing"
	"time"

	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// TestSetHeartbeatTimeoutPausedResumeTouchesHB 恢复 pause 须 touchHB，
// 避免配网刚结束因暂停期间的自然静默立刻误判超时。
func TestSetHeartbeatTimeoutPausedResumeTouchesHB(t *testing.T) {
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
	defer sc.Close()

	before := c.LastPeerActivity()
	if before.IsZero() {
		t.Fatal("Dial 后应已有 LastPeerActivity")
	}
	time.Sleep(25 * time.Millisecond)

	c.SetHeartbeatTimeoutPaused(true)
	c.SetHeartbeatTimeoutPaused(false)

	after := c.LastPeerActivity()
	if !after.After(before) {
		t.Fatalf("恢复 hbPause 应 touchHB: before=%v after=%v", before, after)
	}
}
