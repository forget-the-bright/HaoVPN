package clientapp

import (
	"context"
	"errors"
	"testing"

	"haovpn/internal/config"
	"haovpn/internal/tunnel"
)

// TestApplyPolicyAbortedAtStart Stop 取消的 ctx 须在 via/ICS 前立即返回，避免空跑。
func TestApplyPolicyAbortedAtStart(t *testing.T) {
	rt := &runtime{cfg: &config.ClientConfig{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := rt.applyPolicy(ctx, tunnel.HandshakePolicy{VPNIP: "10.88.0.2"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want Canceled", err)
	}
}

// TestSetupViaExitAbortedBeforeSetup local_lans 非空但 ctx 已取消时，不得进入 Stack.Setup。
func TestSetupViaExitAbortedBeforeSetup(t *testing.T) {
	rt := &runtime{cfg: &config.ClientConfig{LocalLANs: []string{"192.168.1.0/24"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	did, err := rt.setupViaExitLocked(ctx, "10.88.0.0/24", "haovpn_client", "10.88.0.2", []string{"192.168.1.0/24"})
	if did {
		t.Fatal("abort 不得 did_setup")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want Canceled", err)
	}
}

// TestIsStoppingSetByStop Stop 置 stopping，Start 清零。
func TestIsStoppingSetByStop(t *testing.T) {
	e := NewEngine(&config.ClientConfig{})
	if e.isStopping() {
		t.Fatal("new engine 不应 stopping")
	}
	e.Stop()
	if !e.isStopping() {
		t.Fatal("Stop 后应 stopping")
	}
}

// TestStopCancelsRunContext Stop 须取消 runCtx（applyPolicy abort 依赖）。
func TestStopCancelsRunContext(t *testing.T) {
	e := NewEngine(&config.ClientConfig{})
	e.cfg.Server.Address = "127.0.0.1:1"
	// 不真正 Start 重连：直接注入 ctx
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.runCtx = ctx
	e.cancel = cancel
	e.mu.Unlock()
	e.Stop()
	if ctx.Err() == nil {
		t.Fatal("Stop 后 runCtx 应已取消")
	}
	if !e.isStopping() {
		t.Fatal("stopping")
	}
}
