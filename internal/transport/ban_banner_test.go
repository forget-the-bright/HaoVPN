package transport_test

import (
	"errors"
	"net"
	"sync"
	"testing"

	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// banProbe 测试用：模拟封禁拒绝并返回 banner。
type banProbe struct{}

func (banProbe) AllowAccept(string) bool { return false }

func (banProbe) CheckAccept(string) (bool, string) {
	return false, transport.BannerIPBanned
}

func (banProbe) OnTransportReadError(string, error) {}

func (banProbe) OnFrameDecodeError(string, int, error) {}

// TestDialRejectsIPBannedBanner 服务端 TLS 前写入 HAOVPN:IP_BANNED 时客户端得 ErrIPBanned。
func TestDialRejectsIPBannedBanner(t *testing.T) {
	cert := testTLSCert(t)
	cfg := transport.DefaultConfig()
	cfg.Probe = banProbe{}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			allow, banner := cfg.Probe.CheckAccept(raw.RemoteAddr().String())
			if !allow && banner != "" {
				_, _ = raw.Write([]byte(banner))
			}
			_ = raw.Close()
		}
	}()
	defer func() {
		_ = ln.Close()
		wg.Wait()
	}()

	clientTLS := security.TLSConfig(cert, false)
	_, err = transport.Dial(addr, clientTLS, cfg, nil, nil)
	if !errors.Is(err, transport.ErrIPBanned) {
		t.Fatalf("expected ErrIPBanned, got %v", err)
	}
}
