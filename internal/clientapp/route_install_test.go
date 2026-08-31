package clientapp

import (
	"strings"
	"testing"

	"haovpn/internal/config"
)

// TestCheckRouteInstallResult 零成功硬失败；部分成功给 warn；全成功无输出。
func TestCheckRouteInstallResult(t *testing.T) {
	if _, err := checkRouteInstallResult(nil, 0, 0); err != nil {
		t.Fatalf("empty desired: %v", err)
	}
	if _, err := checkRouteInstallResult([]string{"10.0.0.0/24"}, 0, 1); err == nil {
		t.Fatal("expected hard fail when ok=0")
	}
	warn, err := checkRouteInstallResult([]string{"10.0.0.0/24", "10.1.0.0/24"}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if warn == "" || !strings.Contains(warn, "部分") {
		t.Fatalf("expected partial warn, got %q", warn)
	}
	warn, err = checkRouteInstallResult([]string{"10.0.0.0/24"}, 1, 0)
	if err != nil || warn != "" {
		t.Fatalf("full ok: warn=%q err=%v", warn, err)
	}
}

// TestStopClearsSessionPriv Stop 须清空会话私钥，避免内存残留。
func TestStopClearsSessionPriv(t *testing.T) {
	e := NewEngine(&config.ClientConfig{})
	e.activeMu.Lock()
	e.sessionPriv = "test-session-private-key"
	e.activeMu.Unlock()
	e.Stop()
	e.activeMu.Lock()
	defer e.activeMu.Unlock()
	if e.sessionPriv != "" {
		t.Fatal("Stop 后 sessionPriv 应为空")
	}
}
