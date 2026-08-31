package transport_test

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

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

// sourceDenyProbe 测试用：源白名单拒绝。
type sourceDenyProbe struct{}

func (sourceDenyProbe) AllowAccept(string) bool { return false }

func (sourceDenyProbe) CheckAccept(string) (bool, string) {
	return false, transport.BannerSourceDenied
}

func (sourceDenyProbe) OnTransportReadError(string, error) {}

func (sourceDenyProbe) OnFrameDecodeError(string, int, error) {}

// TestDialRejectsIPBannedBanner 服务端 TLS 前立即写入 HAOVPN:IP_BANNED 时客户端得 ErrIPBanned。
func TestDialRejectsIPBannedBanner(t *testing.T) {
	dialRejectBanner(t, banProbe{}, 0, transport.ErrIPBanned)
}

// TestDialRejectsIPBannedBannerSlightDelay 短延迟仍应在 peek 窗口内识别封禁。
func TestDialRejectsIPBannedBannerSlightDelay(t *testing.T) {
	dialRejectBanner(t, banProbe{}, 80*time.Millisecond, transport.ErrIPBanned)
}

// TestDialRejectsLateBannerAsPlaintext 超过 peek 窗口的晚到 banner 应变为 ErrPlaintextBeforeTLS（致命停重连，非瞎报封禁）。
func TestDialRejectsLateBannerAsPlaintext(t *testing.T) {
	dialRejectBanner(t, banProbe{}, 400*time.Millisecond, transport.ErrPlaintextBeforeTLS)
}

// TestDialRejectsSourceDeniedBanner 源白名单拒绝码。
func TestDialRejectsSourceDeniedBanner(t *testing.T) {
	dialRejectBanner(t, sourceDenyProbe{}, 0, transport.ErrSourceDenied)
}

func dialRejectBanner(t *testing.T, probe transport.ProbeObserver, writeDelay time.Duration, want error) {
	t.Helper()
	cfg := transport.DefaultConfig()
	cfg.Probe = probe

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
			if !allow {
				if writeDelay > 0 {
					time.Sleep(writeDelay)
				}
				transport.WriteRejectBanner(raw, banner)
			}
			_ = raw.Close()
		}
	}()
	defer func() {
		_ = ln.Close()
		wg.Wait()
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	_, err = transport.Dial(addr, clientTLS, cfg, nil, nil)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
	if errors.Is(want, transport.ErrIPBanned) || errors.Is(want, transport.ErrSourceDenied) || errors.Is(want, transport.ErrPlaintextBeforeTLS) {
		if !transport.IsFatalDialError(err) {
			t.Fatalf("expected fatal dial error for %v", err)
		}
	}
}

// emptyCloseProbe 仅关闭不写 banner（模拟旧行为或闪断）。
type emptyCloseProbe struct{}

func (emptyCloseProbe) AllowAccept(string) bool                   { return false }
func (emptyCloseProbe) CheckAccept(string) (bool, string)         { return false, "" }
func (emptyCloseProbe) OnTransportReadError(string, error)        {}
func (emptyCloseProbe) OnFrameDecodeError(string, int, error)     {}

// TestDialClosedBeforeTLSNotIPBanned 无 banner 的关闭不得误判为封禁。
func TestDialClosedBeforeTLSNotIPBanned(t *testing.T) {
	dialRejectBanner(t, emptyCloseProbe{}, 0, transport.ErrClosedBeforeTLS)
	if transport.IsFatalDialError(transport.ErrClosedBeforeTLS) {
		t.Fatal("closed-before-tls should not be fatal dial (允许重试)")
	}
}
