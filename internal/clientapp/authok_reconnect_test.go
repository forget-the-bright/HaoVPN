package clientapp

import (
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/config"
)

// TestShouldFailFastAfterAuthOKNeverStopsOnOnline 曾鉴权成功后 account_online 不因次数停重连。
func TestShouldFailFastAfterAuthOKNeverStopsOnOnline(t *testing.T) {
	e := NewEngine(&config.ClientConfig{})
	e.SetFailFast(true)
	e.markAuthOK()
	if !e.hasAuthOKOnce() {
		t.Fatal("markAuthOK 后 authOKOnce 应为 true")
	}
	err := auth.ErrAccountAlreadyOnline
	for i := 0; i < 20; i++ {
		if e.ShouldFailFastHandshake(err) {
			t.Fatalf("曾连接后第 %d 次 account_online 不应 fatal", i+1)
		}
	}
}
