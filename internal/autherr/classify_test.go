package autherr_test

import (
	"errors"
	"fmt"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/autherr"
	"haovpn/internal/dialerr"
)

func TestClassifySentinels(t *testing.T) {
	cases := []struct {
		err  error
		want autherr.Category
	}{
		{dialerr.ErrIPBanned, autherr.CategoryIPBanned},
		{auth.ErrAccountAlreadyOnline, autherr.CategoryAccountOnline},
		{fmt.Errorf("wrap: %w", auth.ErrAccountAlreadyOnline), autherr.CategoryAccountOnline},
		{auth.ErrBadCredentials, autherr.CategoryAuthFailed},
		{auth.ErrLoginLocked, autherr.CategoryAuthFailed},
		{auth.ErrAccountDisabled, autherr.CategoryFatalAuth},
		{auth.ErrMustChangePassword, autherr.CategoryFatalAuth},
		{autherr.ErrSourceDenied, autherr.CategorySourceDenied},
		{dialerr.ErrSourceDenied, autherr.CategorySourceDenied},
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
	if !autherr.IsFatalAuth(dialerr.ErrIPBanned) {
		t.Fatal("IP banned is fatal via classify")
	}
	if !autherr.IsFatalAuth(dialerr.ErrPlaintextBeforeTLS) {
		t.Fatal("plaintext before tls should be fatal")
	}
	if !autherr.IsSourceDenied(dialerr.ErrSourceDenied) {
		t.Fatal("dialerr source denied")
	}
}

func TestIsIPBanned(t *testing.T) {
	if !autherr.IsIPBanned(dialerr.ErrIPBanned) {
		t.Fatal("expected banned")
	}
	if autherr.IsIPBanned(errors.New("other")) {
		t.Fatal("unexpected")
	}
}

func TestHandshakeCodeRoundTrip(t *testing.T) {
	cases := []error{
		auth.ErrBadCredentials,
		auth.ErrAccountDisabled,
		auth.ErrLoginLocked,
		auth.ErrMustChangePassword,
		auth.ErrNoVPN,
		auth.ErrAccountAlreadyOnline,
		autherr.ErrSourceDenied,
		dialerr.ErrIPBanned,
	}
	for _, err := range cases {
		code := autherr.HandshakeCode(err)
		got := autherr.FromHandshakeCode(code)
		if got == nil || !errors.Is(err, got) && !errors.Is(got, err) {
			// ErrSourceDenied 与 dialerr 同一枚；其余应可 Is
			if !errors.Is(got, err) && !errors.Is(err, got) {
				t.Fatalf("code %q round-trip: in=%v out=%v", code, err, got)
			}
		}
	}
	if autherr.FromHandshakeCode("") != nil {
		t.Fatal("empty code")
	}
	if autherr.FromHandshakeCode("unknown_xyz") != nil {
		t.Fatal("unknown code")
	}
}
