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

// tlsFailProbe 记录 TLS 握手失败回调。
type tlsFailProbe struct {
	mu    sync.Mutex
	calls []string
}

func (p *tlsFailProbe) AllowAccept(string) bool { return true }

func (p *tlsFailProbe) CheckAccept(string) (bool, string) { return true, "" }

func (p *tlsFailProbe) OnTransportReadError(remote string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, remote+":"+err.Error())
}

func (p *tlsFailProbe) OnFrameDecodeError(string, int, error) {}

func (p *tlsFailProbe) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// TestServerTLSHandshakeFailureRecordsProbe TLS Accept 握手失败时须回调 Probe（HTTPS 扫描落库）。
func TestServerTLSHandshakeFailureRecordsProbe(t *testing.T) {
	cert := testTLSCert(t)
	probe := &tlsFailProbe{}
	cfg := transport.DefaultConfig()
	cfg.Probe = probe

	srv, err := transport.ListenTLS("127.0.0.1:0", security.TLSConfig(cert, true), cfg, func(*transport.Conn) {
		t.Fatal("不应建立连接")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	addr := srv.Addr().String()
	raw, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// 发送非 TLS 垃圾数据，触发握手失败
	_, _ = raw.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
	_ = raw.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probe.count() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected OnTransportReadError after TLS handshake failure")
}

// TestServerValidTLSHandshakeDoesNotRecordProbeError 正常 TLS 握手不应误记探针错误。
func TestServerValidTLSHandshakeDoesNotRecordProbeError(t *testing.T) {
	cert := testTLSCert(t)
	probe := &tlsFailProbe{}
	cfg := transport.DefaultConfig()
	cfg.Probe = probe

	connected := make(chan struct{}, 1)
	srv, err := transport.ListenTLS("127.0.0.1:0", security.TLSConfig(cert, true), cfg, func(c *transport.Conn) {
		connected <- struct{}{}
		_ = c.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	clientTLS := security.TLSConfig(cert, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := tls.Dial("tcp", srv.Addr().String(), clientTLS)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()

	select {
	case <-connected:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not accept valid TLS")
	}
	if probe.count() != 0 {
		t.Fatalf("expected no probe errors, got %d", probe.count())
	}
}

// 确保 probe 实现 transport.ProbeObserver
var _ transport.ProbeObserver = (*tlsFailProbe)(nil)
