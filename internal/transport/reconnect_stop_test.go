package transport

import (
	"crypto/tls"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// TestReconnectClientStopWaitsAndSkipsOnConnect Dial 阻塞时 Stop 须返回且不再 onConnect。
func TestReconnectClientStopWaitsAndSkipsOnConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// 接受 TCP 后不完成 TLS，使客户端 Dial 卡在握手
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c
			// 故意不读写，保持 Dial 挂起直至 Stop 关连接或超时
			time.Sleep(2 * time.Second)
			_ = c.Close()
		}
	}()

	var onConnectN atomic.Int32
	cfg := DefaultConfig()
	cfg.DialTimeout = 5 * time.Second
	cfg.ReconnectInitial = 50 * time.Millisecond
	cfg.ReconnectMax = 100 * time.Millisecond

	tlsCfg := &tls.Config{InsecureSkipVerify: true, ServerName: "localhost"}
	rc := NewReconnectClient(ln.Addr().String(), tlsCfg, cfg, nil, func(c *Conn) {
		onConnectN.Add(1)
		c.Close()
	})
	rc.Start()
	time.Sleep(80 * time.Millisecond) // 进入 Dial
	start := time.Now()
	rc.Stop()
	elapsed := time.Since(start)
	if elapsed > stopWaitTimeout {
		t.Fatalf("Stop 过久 elapsed=%s", elapsed)
	}
	if onConnectN.Load() != 0 {
		t.Fatalf("Stop 后不应 onConnect，got %d", onConnectN.Load())
	}
}

// TestReconnectClientStopBeforeStart 未 Start 时 Stop 不得阻塞。
func TestReconnectClientStopBeforeStart(t *testing.T) {
	rc := NewReconnectClient("127.0.0.1:1", &tls.Config{InsecureSkipVerify: true}, DefaultConfig(), nil, nil)
	done := make(chan struct{})
	go func() {
		rc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 在未 Start 时不应阻塞")
	}
}

// TestReconnectClientDoubleStop 多次 Stop 安全。
func TestReconnectClientDoubleStop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		time.Sleep(500 * time.Millisecond)
		_ = c.Close()
	}()
	cfg := DefaultConfig()
	cfg.DialTimeout = 2 * time.Second
	rc := NewReconnectClient(ln.Addr().String(), &tls.Config{InsecureSkipVerify: true}, cfg, nil, nil)
	rc.Start()
	time.Sleep(30 * time.Millisecond)
	rc.Stop()
	rc.Stop()
}
