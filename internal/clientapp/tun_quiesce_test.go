package clientapp

import (
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
)

// TestTUNUploadReadyRequiresStateConnected Connected 前即使已有 crypto 也禁止上送。
//
// 对应 local_lans/ICS 长配网窗口：activeConn/cryptoSess 已挂、State 仍为 Connecting。
func TestTUNUploadReadyRequiresStateConnected(t *testing.T) {
	eng := NewEngine(&config.ClientConfig{})
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := crypto.NewSession(kp.PrivateKey, peer.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	eng.activeMu.Lock()
	eng.cryptoSess = sess
	// activeConn 故意为 nil：即便补上，State 非 Connected 也须拒绝
	eng.activeMu.Unlock()

	eng.mu.Lock()
	eng.state = StateConnecting
	eng.mu.Unlock()
	if _, _, ok := eng.tunUploadReady(); ok {
		t.Fatal("StateConnecting 时不应允许 TUN 上送")
	}

	eng.mu.Lock()
	eng.state = StateReconnecting
	eng.mu.Unlock()
	if _, _, ok := eng.tunUploadReady(); ok {
		t.Fatal("StateReconnecting 时不应允许 TUN 上送")
	}

	eng.mu.Lock()
	eng.state = StateConnected
	eng.mu.Unlock()
	// Connected 但无 conn → conn_mismatch
	if _, _, ok := eng.tunUploadReady(); ok {
		t.Fatal("无 activeConn 时不应允许上送")
	}
}
