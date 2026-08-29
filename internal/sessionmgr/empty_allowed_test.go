package sessionmgr_test

import (
	"testing"

	"haovpn/internal/crypto"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
)

// TestRegisterVPNRejectsEmptyAllowed 空 AllowedIPs 须拒绝注册。
func TestRegisterVPNRejectsEmptyAllowed(t *testing.T) {
	store, err := persist.Open(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := sessionmgr.New(store)
	kp, _ := crypto.GenerateKeyPair()
	sess, _ := crypto.NewSession(kp.PrivateKey, kp.PublicKey)
	u := &persist.User{ID: 1, Username: "u", PublicKey: kp.PublicKey, VPNIP: "10.88.0.2", Enabled: true}
	err = m.RegisterVPN(u, nil, nil, sess, "1.2.3.4:1", sessionmgr.PeerReg{})
	if err == nil {
		t.Fatal("expected reject empty allowed")
	}
	if m.OnlineCount() != 0 {
		t.Fatal("must not stay online")
	}
}
