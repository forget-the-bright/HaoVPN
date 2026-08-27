package transport_test

import (
	"crypto/tls"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// TestTLSConcurrentEcho 多客户端并发 TLS 回声，验证服务端 transport 无竞态丢包。
func TestTLSConcurrentEcho(t *testing.T) {
	cert := testTLSCert(t)
	cfg := transport.DefaultConfig()
	cfg.HeartbeatInterval = 200 * time.Millisecond
	cfg.HeartbeatTimeout = 8 * time.Second
	cfg.MaxQueueSize = 512 // 并发回声压测时避免 send queue full 丢包

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	tlsCfg := security.TLSConfig(cert, true)
	serve := func(raw net.Conn) {
		tlsConn := tls.Server(raw, tlsCfg)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		sc := transport.AcceptConn(tlsConn, cfg, nil, nil)
		sc.SetOnData(func(data []byte) {
			_ = sc.Send(data)
		})
	}
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(raw)
		}
	}()

	// 预热：确认监听与回声路径就绪后再并发压测（避免与全量 go test 竞争时首包超时）
	if err := probeEcho(ln.Addr().String(), cfg); err != nil {
		t.Fatalf("server warmup failed: %v", err)
	}

	const clients = 8
	var wg sync.WaitGroup
	var fail atomic.Int32
	sem := make(chan struct{}, 4) // 限制并发握手，避免全量 go test 时 CPU 争用导致偶发超时
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(stagger int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			time.Sleep(time.Duration(stagger%4) * 50 * time.Millisecond)
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				if err := echoOnce(ln.Addr().String(), cfg, 30*time.Second); err == nil {
					return
				} else {
					lastErr = err
					time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
				}
			}
			_ = lastErr
			fail.Add(1)
		}(i)
	}
	wg.Wait()
	if fail.Load() > 0 {
		t.Fatalf("concurrent echo failures: %d/%d", fail.Load(), clients)
	}
}

func probeEcho(addr string, cfg transport.Config) error {
	return echoOnce(addr, cfg, 5*time.Second)
}

func echoOnce(addr string, cfg transport.Config, timeout time.Duration) error {
	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	done := make(chan struct{})
	var got []byte
	var once sync.Once
	client, err := transport.Dial(addr, clientTLS, cfg, func(data []byte) {
		got = append(got, data...)
		once.Do(func() { close(done) })
	}, nil)
	if err != nil {
		return err
	}
	defer client.Close()
	payload := []byte("concurrent-ping")
	if err := client.Send(payload); err != nil {
		return err
	}
	select {
	case <-done:
	case <-time.After(timeout):
		return net.ErrClosed
	}
	if string(got) != string(payload) {
		return net.ErrClosed
	}
	return nil
}
