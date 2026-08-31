package clientapp

import (
	"testing"
	"time"
)

// TestWaitDNSReadyIP 字面 IP 无需 Lookup，应立即 ok。
func TestWaitDNSReadyIP(t *testing.T) {
	if !WaitDNSReady("127.0.0.1:8443", time.Second) {
		t.Fatal("IP 地址应视为已 settle")
	}
}

// TestWaitDNSReadyLocalhost localhost 在多数环境可解析。
func TestWaitDNSReadyLocalhost(t *testing.T) {
	ok := WaitDNSReady("localhost:8443", 2*time.Second)
	if !ok {
		t.Log("localhost 解析失败（环境 DNS），不视为测试失败")
	}
}

// TestWaitDNSReadyTimeout 不可解析主机名应在超时后返回 false（不无限阻塞）。
func TestWaitDNSReadyTimeout(t *testing.T) {
	start := time.Now()
	ok := WaitDNSReady("no-such-host.invalid.haovpn-test:8443", 400*time.Millisecond)
	elapsed := time.Since(start)
	if ok {
		t.Fatal("假主机不应 settle 成功")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("超时等待过长 elapsed=%s", elapsed)
	}
}

// TestHardRestartNilOldNilCfg 空配置须明确错误。
func TestHardRestartNilCfg(t *testing.T) {
	_, err := HardRestart(nil, nil, Credentials{})
	if err == nil {
		t.Fatal("expected error for nil cfg")
	}
}
