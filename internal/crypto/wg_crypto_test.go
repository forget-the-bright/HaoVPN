package crypto_test

import (
	"bytes"
	"testing"

	"haovpn/internal/crypto"
)

// TestCrossSessionRoundTrip 验证双方各自 NewSession 后可互通加解密（数据面正确性）。
func TestCrossSessionRoundTrip(t *testing.T) {
	server, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	// 服务端：本端私钥 + 客户端公钥
	sSrv, err := crypto.NewSession(server.PrivateKey, client.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	// 客户端：本端私钥 + 服务端公钥
	sCli, err := crypto.NewSession(client.PrivateKey, server.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	plain := []byte("vpn-ip-packet-payload-12345")
	enc, err := sCli.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := sSrv.Decrypt(enc)
	if err != nil {
		t.Fatalf("服务端解密客户端密文失败（密钥不对称）: %v", err)
	}
	if !bytes.Equal(plain, dec) {
		t.Fatal("明文不匹配")
	}

	enc2, err := sSrv.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	dec2, err := sCli.Decrypt(enc2)
	if err != nil {
		t.Fatalf("客户端解密服务端密文失败: %v", err)
	}
	if !bytes.Equal(plain, dec2) {
		t.Fatal("回程明文不匹配")
	}
}

func TestInvalidKeyLength(t *testing.T) {
	_, err := crypto.ParsePrivateKey("tooshort")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestReplayRejected 同一密文第二次解密必须失败（防重放窗口）。
func TestReplayRejected(t *testing.T) {
	server, _ := crypto.GenerateKeyPair()
	client, _ := crypto.GenerateKeyPair()
	sSrv, err := crypto.NewSession(server.PrivateKey, client.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sCli, err := crypto.NewSession(client.PrivateKey, server.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := sCli.Encrypt([]byte("replay-test-payload"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sSrv.Decrypt(enc); err != nil {
		t.Fatalf("first decrypt: %v", err)
	}
	if _, err := sSrv.Decrypt(enc); err == nil {
		t.Fatal("replay must be rejected")
	}
}
