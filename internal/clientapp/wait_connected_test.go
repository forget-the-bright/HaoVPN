package clientapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"haovpn/internal/clientapp"
	"haovpn/internal/config"
)

// TestWaitConnectedFailFastSignals 首次失败须立刻唤醒 WaitConnected（GUI 登录不卡死）。
func TestWaitConnectedFailFastSignals(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.ApplyDefaults()
	cfg.Server.Address = "127.0.0.1:1" // 不可达端口，快速 dial 失败
	cfg.Server.TLS.InsecureSkipVerify = true
	eng := clientapp.NewEngine(cfg)
	eng.SetFailFast(true)
	eng.SetCredentials(clientapp.Credentials{Username: "u", Password: "p"})
	if err := eng.Start(); err != nil {
		t.Fatal(err)
	}
	defer eng.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := eng.WaitConnected(ctx)
	if err == nil {
		t.Fatal("期望拨号失败")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("不应等到 ctx 超时，应尽快返回拨号错误: %v", err)
	}
}
