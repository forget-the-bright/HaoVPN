package clientapp_test

import (
	"errors"
	"testing"

	"haovpn/internal/clientapp"
	"haovpn/internal/transport"
)

func TestFormatDialErrorIPBanned(t *testing.T) {
	msg := clientapp.FormatDialError(transport.ErrIPBanned)
	if msg == "" || msg == transport.ErrIPBanned.Error() {
		t.Fatalf("expected friendly message, got %q", msg)
	}
	if msg != "您的 IP 已被服务端封禁，无法连接。请联系管理员在管理台「探针」页解封或加入豁免名单。" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestIsIPBannedDialErrorFatal(t *testing.T) {
	if !clientapp.IsIPBannedDialError(transport.ErrIPBanned) {
		t.Fatal("expected banned dial error")
	}
	if !clientapp.IsFatalHandshakeError(transport.ErrIPBanned) {
		t.Fatal("IP banned should be fatal handshake")
	}
	if clientapp.IsIPBannedDialError(errors.New("other")) {
		t.Fatal("unexpected match")
	}
}
