package tunnel_test

import (
	"encoding/json"
	"errors"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/autherr"
	"haovpn/internal/tunnel"
)

// TestEncodeHandshakeErrCodeJSON 验证 handshake_err 含 code 字段。
func TestEncodeHandshakeErrCodeJSON(t *testing.T) {
	b, err := tunnel.EncodeHandshakeErrCode(autherr.CodeBadCredentials, auth.ErrBadCredentials.Error())
	if err != nil {
		t.Fatal(err)
	}
	var resp tunnel.HandshakeResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "handshake_err" || resp.Code != autherr.CodeBadCredentials {
		t.Fatalf("got %+v", resp)
	}
	if !errors.Is(autherr.FromHandshakeCode(resp.Code), auth.ErrBadCredentials) {
		t.Fatal("code should map to ErrBadCredentials")
	}
}

// TestHandshakeErrFromResponseViaParse 新旧互操作：有 code / 无 code。
func TestHandshakeErrFromResponseViaParse(t *testing.T) {
	// 有 code：应可 errors.Is
	withCode, _ := tunnel.EncodeHandshakeErrCode(autherr.CodeAccountOnline, auth.ErrAccountAlreadyOnline.Error())
	resp, err := tunnel.ParseHandshakeResponse(withCode)
	if err != nil {
		t.Fatal(err)
	}
	got := autherr.FromHandshakeCode(resp.Code)
	if !errors.Is(got, auth.ErrAccountAlreadyOnline) {
		t.Fatalf("want account online, got %v", got)
	}

	// 无 code：旧服务端仅文案
	legacy, _ := tunnel.EncodeHandshakeErr("用户名或密码错误")
	resp2, err := tunnel.ParseHandshakeResponse(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Code != "" {
		t.Fatalf("legacy should have empty code, got %q", resp2.Code)
	}
	if autherr.Classify(errors.New(resp2.Error)) != autherr.CategoryAuthFailed {
		t.Fatal("legacy text should classify as auth failed")
	}

	// source_denied code
	src, _ := tunnel.EncodeHandshakeErrCode(autherr.CodeSourceDenied, autherr.ErrSourceDenied.Error())
	resp3, _ := tunnel.ParseHandshakeResponse(src)
	if !errors.Is(autherr.FromHandshakeCode(resp3.Code), autherr.ErrSourceDenied) {
		t.Fatal("source denied code")
	}
}
