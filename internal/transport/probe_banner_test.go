package transport

import (
	"errors"
	"strings"
	"testing"
)

func TestClassifyTLSHandshakeErrPlaintext(t *testing.T) {
	raw := errors.New("tls: first record does not look like a TLS handshake")
	got := classifyTLSHandshakeErr(raw)
	if !errors.Is(got, ErrPlaintextBeforeTLS) {
		t.Fatalf("expected ErrPlaintextBeforeTLS, got %v", got)
	}
}

func TestClassifyRejectBannerLine(t *testing.T) {
	if !errors.Is(classifyRejectBannerLine("HAOVPN:IP_BANNED\r\n"), ErrIPBanned) {
		t.Fatal("ip banned")
	}
	if !errors.Is(classifyRejectBannerLine("HAOVPN:SOURCE_DENIED\r\n"), ErrSourceDenied) {
		t.Fatal("source denied")
	}
	if classifyRejectBannerLine("HTTP/1.1 403") == nil {
		t.Fatal("expected unexpected preamble error")
	}
}

func TestClassifyTLSHandshakeErrOther(t *testing.T) {
	raw := errors.New("certificate signed by unknown authority")
	got := classifyTLSHandshakeErr(raw)
	if errors.Is(got, ErrPlaintextBeforeTLS) {
		t.Fatalf("unexpected plaintext classify: %v", got)
	}
	if got.Error() != raw.Error() && !strings.Contains(got.Error(), raw.Error()) {
		t.Fatalf("expected original err preserved, got %v", got)
	}
}

func TestIsFatalDialError(t *testing.T) {
	if !IsFatalDialError(ErrIPBanned) || !IsFatalDialError(ErrSourceDenied) || !IsFatalDialError(ErrPlaintextBeforeTLS) {
		t.Fatal("expected fatals")
	}
	if IsFatalDialError(ErrClosedBeforeTLS) {
		t.Fatal("closed-before-tls must remain retryable")
	}
	if IsFatalDialError(nil) {
		t.Fatal("nil")
	}
}
