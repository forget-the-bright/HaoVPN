package clientapp

import (
	"errors"
	"sync"
	"testing"

	"haovpn/internal/config"
)

// recordingKillSwitch 记录 Enable/Disable/Remove 调用顺序。
type recordingKillSwitch struct {
	mu        sync.Mutex
	steps     []string
	enableErr error
}

func (r *recordingKillSwitch) Supported() error { return nil }

func (r *recordingKillSwitch) Enable([]string) error {
	r.mu.Lock()
	r.steps = append(r.steps, "enable")
	err := r.enableErr
	r.mu.Unlock()
	return err
}

func (r *recordingKillSwitch) Disable() error {
	r.mu.Lock()
	r.steps = append(r.steps, "disable")
	r.mu.Unlock()
	return nil
}

func (r *recordingKillSwitch) Remove() error {
	r.mu.Lock()
	r.steps = append(r.steps, "remove")
	r.mu.Unlock()
	return nil
}

func (r *recordingKillSwitch) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.steps))
	copy(out, r.steps)
	return out
}

// TestProtectThenClearRoutesOrder 断线必须先 Enable 杀开关再清路由。
func TestProtectThenClearRoutesOrder(t *testing.T) {
	ks := &recordingKillSwitch{}
	cfg := &config.ClientConfig{}
	cfg.Security.KillSwitch = true
	e := NewEngine(cfg)
	e.ks = ks
	e.rt.mu.Lock()
	e.rt.allowedCIDRs = []string{"192.168.1.0/24"}
	e.rt.mu.Unlock()
	e.clearRoutesHook = func() {
		ks.mu.Lock()
		ks.steps = append(ks.steps, "clear")
		ks.mu.Unlock()
	}

	e.protectThenClearRoutes()

	got := ks.snapshot()
	if len(got) < 2 || got[0] != "enable" || got[1] != "clear" {
		t.Fatalf("期望 [enable, clear, ...]，得到 %v", got)
	}
	if !e.KillSwitchOK() {
		t.Fatal("Enable 成功后 KillSwitchOK 应为 true")
	}
}

// TestProtectThenClearRoutesNoLeakOnEnableFail Enable 失败禁止清路由。
func TestProtectThenClearRoutesNoLeakOnEnableFail(t *testing.T) {
	ks := &recordingKillSwitch{enableErr: errors.New("wfp denied")}
	cfg := &config.ClientConfig{}
	cfg.Security.KillSwitch = true
	e := NewEngine(cfg)
	e.ks = ks
	e.rt.mu.Lock()
	e.rt.allowedCIDRs = []string{"192.168.1.0/24"}
	e.rt.mu.Unlock()
	cleared := false
	e.clearRoutesHook = func() { cleared = true }

	e.protectThenClearRoutes()

	got := ks.snapshot()
	if len(got) != 1 || got[0] != "enable" {
		t.Fatalf("期望仅 enable，得到 %v", got)
	}
	if cleared {
		t.Fatal("Enable 失败时不得 clearRoutes")
	}
	if e.KillSwitchOK() {
		t.Fatal("Enable 失败时 KillSwitchOK 应为 false")
	}
	if e.LastError() == "" {
		t.Fatal("应设置 LastError 供 GUI 展示")
	}
}
