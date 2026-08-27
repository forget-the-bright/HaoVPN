package transport_test

import (
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// TestProbeMTUEnqueued 握手后 ProbeMTU 须成功入队（对端读循环消费 MTU 帧）。
func TestProbeMTUEnqueued(t *testing.T) {
	cert := testTLSCert(t)
	tlsSrv := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var gotProbe sync.WaitGroup
	gotProbe.Add(1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tc := tls.Server(raw, tlsSrv)
		if err := tc.Handshake(); err != nil {
			return
		}
		cfg := transport.DefaultConfig()
		cfg.MaxQueueSize = 64
		_ = transport.AcceptConn(tc, cfg, func(data []byte) {}, nil)
		// AcceptConn 已启动读循环；MTU probe 在服务端被静默处理（FrameTypeMTUProbe）
		gotProbe.Done()
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	cfg := transport.DefaultConfig()
	cfg.MaxQueueSize = 64
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	select {
	case <-waitDone(&gotProbe):
	case <-time.After(3 * time.Second):
		t.Fatal("server accept timeout")
	}

	if err := conn.ProbeMTU(200); err != nil {
		t.Fatalf("ProbeMTU: %v", err)
	}
}

func waitDone(wg *sync.WaitGroup) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}
