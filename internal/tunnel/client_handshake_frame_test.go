package tunnel_test

import (
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"haovpn/internal/security"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
)

// TestClientHandshakeIgnoresDataFrames Data 密文不得结束握手为 JSON 错误。
func TestClientHandshakeIgnoresDataFrames(t *testing.T) {
	cert := genTestCert(t)
	tlsCfg := security.TLSConfig(cert, true)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer raw.Close()
		tc := tls.Server(raw, tlsCfg)
		if err := tc.Handshake(); err != nil {
			errCh <- err
			return
		}
		sc := transport.AcceptConn(tc, transport.DefaultConfig(), nil, nil)
		defer sc.Close()
		// 先发一帧 Data（模拟脏密文），再发合法 Handshake
		if err := sc.SendRaw(transport.FrameTypeData, []byte{0x00, 0x01, 0x02, 0x03}); err != nil {
			errCh <- err
			return
		}
		time.Sleep(50 * time.Millisecond)
		if err := sc.SendRaw(transport.FrameTypeHandshake, []byte(`{"type":"handshake_err","error":"账号或密码错误","code":"bad_credentials"}`)); err != nil {
			errCh <- err
			return
		}
		time.Sleep(300 * time.Millisecond)
		errCh <- nil
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	conn, err := transport.Dial(ln.Addr().String(), clientTLS, transport.DefaultConfig(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	hs := tunnel.NewClientHandshake()
	_, err = hs.RunAuthWithTimeout(conn, "u", "p", 5*time.Second)
	if err == nil {
		t.Fatal("期望 handshake_err")
	}
	// 若误把 Data 当 JSON，会含 invalid character / 非 JSON
	if strings.Contains(err.Error(), "invalid character") || strings.Contains(err.Error(), "非 JSON") {
		t.Fatalf("Data 帧不应导致 JSON 解析失败: %v", err)
	}
	if !strings.Contains(err.Error(), "账号或密码错误") && !strings.Contains(err.Error(), "bad_credentials") {
		// 允许中文 error 字段或其它包装，但不应是 JSON 解析类
		t.Logf("handshake err (ok if not JSON parse): %v", err)
	}
	select {
	case e := <-errCh:
		if e != nil {
			t.Fatalf("server: %v", e)
		}
	case <-time.After(2 * time.Second):
	}
}
