package dialerr_test

import (
	"errors"
	"fmt"
	"testing"

	"haovpn/internal/dialerr"
)

func TestClassifyRejectBannerLine(t *testing.T) {
	if !errors.Is(dialerr.ClassifyRejectBannerLine("HAOVPN:IP_BANNED\r\n"), dialerr.ErrIPBanned) {
		t.Fatal("IP_BANNED")
	}
	if !errors.Is(dialerr.ClassifyRejectBannerLine("HAOVPN:SOURCE_DENIED\r\n"), dialerr.ErrSourceDenied) {
		t.Fatal("SOURCE_DENIED")
	}
	if dialerr.ClassifyRejectBannerLine("HTTP/1.1 403") == nil {
		t.Fatal("unexpected preamble should error")
	}
}

func TestClassifyTLSHandshakeErr(t *testing.T) {
	raw := errors.New("tls: first record does not look like a TLS handshake")
	got := dialerr.ClassifyTLSHandshakeErr(raw)
	if !errors.Is(got, dialerr.ErrPlaintextBeforeTLS) {
		t.Fatalf("want ErrPlaintextBeforeTLS, got %v", got)
	}
	other := errors.New("connection reset")
	if dialerr.ClassifyTLSHandshakeErr(other) != other {
		t.Fatal("non-bad-record should pass through")
	}
	if dialerr.ClassifyTLSHandshakeErr(nil) != nil {
		t.Fatal("nil")
	}
}

func TestIsFatalDialError(t *testing.T) {
	if !dialerr.IsFatalDialError(dialerr.ErrIPBanned) ||
		!dialerr.IsFatalDialError(dialerr.ErrSourceDenied) ||
		!dialerr.IsFatalDialError(dialerr.ErrPlaintextBeforeTLS) {
		t.Fatal("expected fatal")
	}
	if dialerr.IsFatalDialError(dialerr.ErrClosedBeforeTLS) {
		t.Fatal("closed before tls is retryable")
	}
	if dialerr.IsFatalDialError(nil) {
		t.Fatal("nil")
	}
	wrapped := fmt.Errorf("wrap: %w", dialerr.ErrIPBanned)
	if !dialerr.IsFatalDialError(wrapped) {
		t.Fatal("wrapped should be fatal")
	}
}

func TestIsTLSBadRecordMsg(t *testing.T) {
	if !dialerr.IsTLSBadRecordMsg(errors.New("tls: first record does not look like a TLS handshake")) {
		t.Fatal("expected match")
	}
	if dialerr.IsTLSBadRecordMsg(errors.New("eof")) {
		t.Fatal("unexpected match")
	}
}
