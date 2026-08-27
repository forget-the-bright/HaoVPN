package transport_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/security"
	"haovpn/internal/transport"
)

func init() {
	_ = logger.Init(logger.Config{Level: "error"})
}

func testTLSCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return tlsCert
}

func TestTLSRoundTrip(t *testing.T) {
	cert := testTLSCert(t)
	cfg := transport.DefaultConfig()
	cfg.HeartbeatInterval = 100 * time.Millisecond
	cfg.HeartbeatTimeout = 2 * time.Second

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	tlsCfg := security.TLSConfig(cert, true)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		tlsConn := tls.Server(raw, tlsCfg)
		if err := tlsConn.Handshake(); err != nil {
			t.Errorf("server handshake: %v", err)
			return
		}
		// 必须先 AcceptConn 再 SetOnData，避免回调触发时 conn 尚未赋值（间歇性 echo 超时根因）
		sc := transport.AcceptConn(tlsConn, cfg, nil, nil)
		sc.SetOnData(func(data []byte) {
			_ = sc.Send(data)
		})
		_ = sc
	}()

	clientTLS := security.TLSConfig(tls.Certificate{}, false)
	clientTLS.InsecureSkipVerify = true
	var received []byte
	done := make(chan struct{})
	client, err := transport.Dial(ln.Addr().String(), clientTLS, cfg, func(data []byte) {
		received = append(received, data...)
		close(done)
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	payload := []byte("ping")
	if err := client.Send(payload); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for echo")
	}
	if string(received) != "ping" {
		t.Fatalf("got %q want ping", received)
	}
	wg.Wait()
}
