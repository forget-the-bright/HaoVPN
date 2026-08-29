package clientapp_test

import (
	"errors"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/clientapp"
	"haovpn/internal/config"
)

func TestIsFatalHandshakeError(t *testing.T) {
	if !clientapp.IsFatalHandshakeError(auth.ErrBadCredentials) {
		t.Fatal("bad credentials should be fatal")
	}
	if clientapp.IsFatalHandshakeError(auth.ErrAccountAlreadyOnline) {
		t.Fatal("account online should not be immediately fatal（有限重试）")
	}
	if !clientapp.IsAccountAlreadyOnline(auth.ErrAccountAlreadyOnline) {
		t.Fatal("IsAccountAlreadyOnline")
	}
	if !clientapp.IsFatalHandshakeError(auth.ErrLoginLocked) {
		t.Fatal("login locked should be fatal")
	}
	if !clientapp.IsFatalHandshakeError(errors.New("登录失败次数过多，请稍后再试")) {
		t.Fatal("legacy lockout")
	}
	if !clientapp.IsFatalHandshakeError(errors.New("用户名或密码错误")) {
		t.Fatal("bad password text")
	}
	if clientapp.IsFatalHandshakeError(errors.New("握手超时")) {
		t.Fatal("timeout not fatal")
	}
	if clientapp.IsFatalHandshakeError(nil) {
		t.Fatal("nil")
	}
}

func TestShouldFailFastHandshakeOnlineRetries(t *testing.T) {
	e := clientapp.NewEngine(&config.ClientConfig{})
	err := auth.ErrAccountAlreadyOnline
	for i := 1; i < 40; i++ {
		if e.ShouldFailFastHandshake(err) {
			t.Fatalf("第 %d 次 account_online 不应 fatal", i)
		}
	}
	if !e.ShouldFailFastHandshake(err) {
		t.Fatal("第 40 次应 fatal")
	}
}
