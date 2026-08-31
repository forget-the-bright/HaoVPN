package probedefense_test

import (
	"errors"
	"fmt"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/dialerr"
	"haovpn/internal/probedefense"
)

func TestClassifyHandshakeReject(t *testing.T) {
	if got := probedefense.ClassifyHandshakeReject(auth.ErrBadCredentials); got != probedefense.SigAuthFailed {
		t.Fatalf("auth: %s", got)
	}
	if got := probedefense.ClassifyHandshakeReject(auth.ErrAccountAlreadyOnline); got != probedefense.SigAccountOnline {
		t.Fatalf("online: %s", got)
	}
	wrapped := fmt.Errorf("注册会话失败: %w", auth.ErrAccountAlreadyOnline)
	if got := probedefense.ClassifyHandshakeReject(wrapped); got != probedefense.SigAccountOnline {
		t.Fatalf("wrapped online: %s", got)
	}
	if got := probedefense.ClassifyHandshakeReject(dialerr.ErrSourceDenied); got != probedefense.SigSourceDeny {
		t.Fatalf("source: %s", got)
	}
	if got := probedefense.ClassifyHandshakeReject(errors.New("用户名或密码错误")); got != probedefense.SigAuthFailed {
		t.Fatalf("cn auth: %s", got)
	}
}
