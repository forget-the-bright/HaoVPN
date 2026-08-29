package clientapp

import (
	"context"
	"testing"
	"time"

	"haovpn/internal/config"
)

// TestWaitConnectedUnblocksBeforeStateConnected 鉴权信号可先于 StateConnected 唤醒 WaitConnected。
//
// 对应 GUI：密码通过后即可离登录页，不必等 TUN/路由配完。
func TestWaitConnectedUnblocksBeforeStateConnected(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.ApplyDefaults()
	cfg.Server.TLS.InsecureSkipVerify = true
	eng := NewEngine(cfg)
	eng.setState(StateConnecting)

	go func() {
		time.Sleep(20 * time.Millisecond)
		// 模拟鉴权成功、applyPolicy 尚未完成
		eng.signalFirstResult(nil)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := eng.WaitConnected(ctx); err != nil {
		t.Fatalf("WaitConnected: %v", err)
	}
	if eng.State() != StateConnecting {
		t.Fatalf("期望仍为 Connecting（数据面未就绪）, got %v", eng.State())
	}
}
