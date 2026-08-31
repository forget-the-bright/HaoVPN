package transport

import (
	"testing"
	"time"

	"haovpn/internal/safeutil"
)

// TestEffectiveDialTimeoutDefault 未配置时须为 3s（避免旧默认 10s）。
func TestEffectiveDialTimeoutDefault(t *testing.T) {
	c := Config{}
	if got := c.EffectiveDialTimeout(); got != 3*time.Second {
		t.Fatalf("got %v", got)
	}
	c.DialTimeout = 5 * time.Second
	if got := c.EffectiveDialTimeout(); got != 5*time.Second {
		t.Fatalf("got %v", got)
	}
}

// TestEffectiveReconnectMaxDefault 退避上限默认 3s。
func TestEffectiveReconnectMaxDefault(t *testing.T) {
	c := DefaultConfig()
	if c.EffectiveReconnectMax() != 3*time.Second {
		t.Fatalf("max=%v", c.EffectiveReconnectMax())
	}
	c.ReconnectMax = 8 * time.Second
	if c.EffectiveReconnectMax() != 8*time.Second {
		t.Fatalf("max=%v", c.EffectiveReconnectMax())
	}
}

// TestAfterDisconnectPause 断线后立即重拨停顿须远小于旧退避。
func TestAfterDisconnectPause(t *testing.T) {
	p := AfterDisconnectPause()
	if p <= 0 || p >= 500*time.Millisecond {
		t.Fatalf("pause=%v want (0,500ms)", p)
	}
}

// TestBackoffCapLogic 用 safeutil.ExpBackoff 模拟失败退避不超过 max（与 ReconnectClient.loop 一致）。
func TestBackoffCapLogic(t *testing.T) {
	cfg := DefaultConfig()
	backoff := cfg.EffectiveReconnectInitial()
	max := cfg.EffectiveReconnectMax()
	for i := 0; i < 10; i++ {
		backoff = safeutil.ExpBackoff(backoff, max)
	}
	if backoff != max {
		t.Fatalf("backoff=%v max=%v", backoff, max)
	}
}
