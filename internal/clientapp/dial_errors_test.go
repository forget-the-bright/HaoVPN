package clientapp_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"haovpn/internal/autherr"
	"haovpn/internal/clientapp"
	"haovpn/internal/dialerr"
)

func TestFormatDialErrorIPBanned(t *testing.T) {
	msg := clientapp.FormatDialError(dialerr.ErrIPBanned)
	if !strings.Contains(msg, "封禁") {
		t.Fatalf("expected ban message, got %q", msg)
	}
}

func TestFormatDialErrorSourceDenied(t *testing.T) {
	msg := clientapp.FormatDialError(dialerr.ErrSourceDenied)
	if !strings.Contains(msg, "白名单") {
		t.Fatalf("expected source denied message, got %q", msg)
	}
}

func TestFormatDialErrorPlaintext(t *testing.T) {
	err := fmt.Errorf("%w: %v", dialerr.ErrPlaintextBeforeTLS, errors.New("tls: first record does not look like a TLS handshake"))
	msg := clientapp.FormatDialError(err)
	if !strings.Contains(msg, "封禁") || !strings.Contains(msg, "端口") {
		t.Fatalf("expected dual-cause plaintext message, got %q", msg)
	}
}

func TestFormatDialErrorClosedBeforeTLS(t *testing.T) {
	msg := clientapp.FormatDialError(dialerr.ErrClosedBeforeTLS)
	if msg == "" || msg == dialerr.ErrClosedBeforeTLS.Error() {
		t.Fatalf("expected friendly closed message, got %q", msg)
	}
}

func TestIsFatalDialErrorKinds(t *testing.T) {
	if !autherr.IsIPBanned(dialerr.ErrIPBanned) {
		t.Fatal("expected banned dial error")
	}
	if !clientapp.IsFatalHandshakeError(dialerr.ErrIPBanned) {
		t.Fatal("IP banned should be fatal handshake")
	}
	if !clientapp.IsFatalDialError(dialerr.ErrPlaintextBeforeTLS) {
		t.Fatal("plaintext before tls should be fatal dial")
	}
	if !clientapp.IsFatalDialError(dialerr.ErrSourceDenied) {
		t.Fatal("source denied should be fatal dial")
	}
	if clientapp.IsFatalDialError(dialerr.ErrClosedBeforeTLS) {
		t.Fatal("closed-before-tls should allow reconnect")
	}
	if autherr.IsIPBanned(errors.New("other")) {
		t.Fatal("unexpected match")
	}
}
