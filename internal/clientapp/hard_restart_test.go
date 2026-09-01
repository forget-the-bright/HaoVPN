package clientapp

import (
	"errors"
	"testing"
	"time"

	"haovpn/internal/config"
)

// TestWaitDNSReadyIP 字面 IP 无需 Lookup，应立即 ok。
func TestWaitDNSReadyIP(t *testing.T) {
	if !waitDNSReady("127.0.0.1:8443", time.Second) {
		t.Fatal("IP 地址应视为已 settle")
	}
}

// TestWaitDNSReadyLocalhost localhost 在多数环境可解析。
func TestWaitDNSReadyLocalhost(t *testing.T) {
	ok := waitDNSReady("localhost:8443", 2*time.Second)
	if !ok {
		t.Log("localhost 解析失败（环境 DNS），不视为测试失败")
	}
}

// TestWaitDNSReadyTimeout 不可解析主机名应在超时后返回 false（不无限阻塞）。
func TestWaitDNSReadyTimeout(t *testing.T) {
	start := time.Now()
	ok := waitDNSReady("no-such-host.invalid.haovpn-test:8443", 400*time.Millisecond)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("假主机不应 settle 成功")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("超时等待过长 elapsed=%s", elapsed)
	}
}

// TestHardRestartNilCfg 空配置须明确错误。
func TestHardRestartNilCfg(t *testing.T) {
	_, err := HardRestart(nil, nil, Credentials{}, nil)
	if err == nil {
		t.Fatal("expected error for nil cfg")
	}
}

// TestHardRestartAbortBeforeStart abort 在 Stop 后立即触发，不得 Start。
func TestHardRestartAbortBeforeStart(t *testing.T) {
	cfg := &config.ClientConfig{}
	eng, err := HardRestart(nil, cfg, Credentials{}, func() bool { return true })
	if !errors.Is(err, ErrHardRestartAborted) {
		t.Fatalf("err=%v want ErrHardRestartAborted", err)
	}
	if eng != nil {
		t.Fatal("aborted 不得返回 eng")
	}
}

// TestWaitDNSReadyAbortMidSettle abort 在 settle 中触发须尽快返回，不得拖满 timeout。
func TestWaitDNSReadyAbortMidSettle(t *testing.T) {
	n := 0
	start := time.Now()
	ok := waitDNSReadyAbort("no-such-host.invalid.haovpn-test:8443", 3*time.Second, func() bool {
		n++
		return n >= 2 // 第二轮起 abort
	})
	elapsed := time.Since(start)
	if ok {
		t.Fatal("abort 后不应 ok")
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("abort 后仍阻塞过久 elapsed=%s", elapsed)
	}
}

// TestHardRestartAbortDuringDNS settle 中 abort 须返回 ErrHardRestartAborted。
func TestHardRestartAbortDuringDNS(t *testing.T) {
	cfg := &config.ClientConfig{}
	cfg.Server.Address = "no-such-host.invalid.haovpn-test:8443"
	n := 0
	eng, err := HardRestart(nil, cfg, Credentials{}, func() bool {
		n++
		// after_stop 检查为第 1 次（false）；settle 内再调 abort 为 true
		return n >= 2
	})
	if !errors.Is(err, ErrHardRestartAborted) {
		t.Fatalf("err=%v want ErrHardRestartAborted", err)
	}
	if eng != nil {
		t.Fatal("aborted 不得返回 eng")
	}
}
