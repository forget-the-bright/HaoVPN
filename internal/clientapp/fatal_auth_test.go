package clientapp_test

import (
	"errors"
	"testing"

	"haovpn/internal/clientapp"
)

func TestIsFatalHandshakeError(t *testing.T) {
	if !clientapp.IsFatalHandshakeError(errors.New("用户名或密码错误")) {
		t.Fatal("密码错误应为致命")
	}
	if !clientapp.IsFatalHandshakeError(errors.New("该账号已在其他设备在线")) {
		t.Fatal("已在线应为致命")
	}
	if clientapp.IsFatalHandshakeError(errors.New("握手超时")) {
		t.Fatal("超时不应视为致命鉴权错误")
	}
	if clientapp.IsFatalHandshakeError(nil) {
		t.Fatal("nil 非致命")
	}
}
