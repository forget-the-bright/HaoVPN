package autherr_test

import (
	"errors"
	"fmt"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/autherr"
	"haovpn/internal/transport"
)

func TestClassifySentinels(t *testing.T) {
	cases := []struct {
		err  error
		want autherr.Category
	}{
		{transport.ErrIPBanned, autherr.CategoryIPBanned},
		{auth.ErrAccountAlreadyOnline, autherr.CategoryAccountOnline},
		{fmt.Errorf("wrap: %w", auth.ErrAccountAlreadyOnline), autherr.CategoryAccountOnline},
		{auth.ErrBadCredentials, autherr.CategoryAuthFailed},
		{auth.ErrLoginLocked, autherr.CategoryAuthFailed},
		{auth.ErrAccountDisabled, autherr.CategoryFatalAuth},
		{auth.ErrMustChangePassword, autherr.CategoryFatalAuth},
		{autherr.ErrSourceDenied, autherr.CategorySourceDenied},
		{errors.New("用户名或密码错误"), autherr.CategoryAuthFailed},
		{errors.New("已在其他设备在线"), autherr.CategoryAccountOnline},
		{errors.New("不在 tunnel_allowed_source_ips"), autherr.CategorySourceDenied},
	}
	for _, tc := range cases {
		if got := autherr.Classify(tc.err); got != tc.want {
			t.Fatalf("Classify(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsFatalAuth(t *testing.T) {
	if !autherr.IsFatalAuth(auth.ErrBadCredentials) {
		t.Fatal("bad creds should be fatal")
	}
	if autherr.IsFatalAuth(auth.ErrAccountAlreadyOnline) {
		t.Fatal("account online is not immediate fatal")
	}
	if !autherr.IsFatalAuth(transport.ErrIPBanned) {
		t.Fatal("IP banned is fatal via classify")
	}
}

func TestIsIPBanned(t *testing.T) {
	if !autherr.IsIPBanned(transport.ErrIPBanned) {
		t.Fatal("expected banned")
	}
	if autherr.IsIPBanned(errors.New("other")) {
		t.Fatal("unexpected")
	}
}
