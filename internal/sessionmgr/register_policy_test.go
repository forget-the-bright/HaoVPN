package sessionmgr_test

import (
	"errors"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

type nopConn struct{}

func (nopConn) Send([]byte) error     { return nil }
func (nopConn) Close() error          { return nil }
func (nopConn) RemoteAddr() string    { return "1.2.3.4:1" }

// TestRegisterVPNRejectSecond 默认策略下第二端须被拒绝且旧会话保持。
func TestRegisterVPNRejectSecond(t *testing.T) {
	m := sessionmgr.New(nil)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 7, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.2", Enabled: true}
	c1, c2 := nopConn{}, nopConn{}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, c1, sess, "1.1.1.1:1"); err != nil {
		t.Fatalf("首连应成功: %v", err)
	}
	err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, c2, sess, "2.2.2.2:2")
	if !errors.Is(err, sessionmgr.ErrAccountAlreadyOnline) {
		t.Fatalf("第二端应 ErrAccountAlreadyOnline，got %v", err)
	}
	if m.OnlineCount() != 1 {
		t.Fatalf("旧会话应保持在线, online=%d", m.OnlineCount())
	}
}

// TestRegisterVPNKickPrevious 策略 kick_previous 时新连接替换旧会话。
func TestRegisterVPNKickPrevious(t *testing.T) {
	m := sessionmgr.New(nil)
	m.SetSessionPolicy(config.SessionPolicyKickPrevious)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 8, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.3", Enabled: true}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, nopConn{}, sess, "1.1.1.1:1"); err != nil {
		t.Fatalf("首连: %v", err)
	}
	if err := m.RegisterVPN(u, []string{"10.88.0.0/24"}, nopConn{}, sess, "2.2.2.2:2"); err != nil {
		t.Fatalf("kick_previous 第二端应成功: %v", err)
	}
	if m.OnlineCount() != 1 {
		t.Fatalf("仍应仅 1 在线, online=%d", m.OnlineCount())
	}
}
