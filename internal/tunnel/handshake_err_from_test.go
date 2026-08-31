package tunnel

import (
	"errors"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/autherr"
)

func TestHandshakeErrFromResponsePreservesSentinel(t *testing.T) {
	err := handshakeErrFromResponse(HandshakeResponse{
		Type:  "handshake_err",
		Code:  autherr.CodeBadCredentials,
		Error: auth.ErrBadCredentials.Error(),
	})
	if !errors.Is(err, auth.ErrBadCredentials) {
		t.Fatalf("got %v", err)
	}

	// 有 code + 不同文案：仍可 Is
	err2 := handshakeErrFromResponse(HandshakeResponse{
		Type:  "handshake_err",
		Code:  autherr.CodeLoginLocked,
		Error: "自定义锁定说明",
	})
	if !errors.Is(err2, auth.ErrLoginLocked) {
		t.Fatalf("wrapped should still Is: %v", err2)
	}

	// 无 code：纯文案
	err3 := handshakeErrFromResponse(HandshakeResponse{
		Type:  "handshake_err",
		Error: "用户名或密码错误",
	})
	if errors.Is(err3, auth.ErrBadCredentials) {
		t.Fatal("legacy string-only cannot Is sentinel (expected)")
	}
	if err3.Error() != "用户名或密码错误" {
		t.Fatalf("got %q", err3.Error())
	}
}
