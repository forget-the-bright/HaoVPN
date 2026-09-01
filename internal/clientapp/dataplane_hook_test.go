package clientapp

import (
	"sync"
	"testing"
	"time"

	"haovpn/internal/config"
)

// TestDataplaneFailedInvokesCallback 验证 OnDataplaneFailed 在数据面失败时被调用。
func TestDataplaneFailedInvokesCallback(t *testing.T) {
	eng := NewEngine(&config.ClientConfig{})
	var mu sync.Mutex
	var got string
	done := make(chan struct{})
	AttachDataplaneHook(eng, func(msg string) {
		mu.Lock()
		got = msg
		mu.Unlock()
		close(done)
	})
	eng.dataplaneFailed(nil, "应用服务端策略失败: 测试")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("回调未触发")
	}
	mu.Lock()
	defer mu.Unlock()
	if got != "应用服务端策略失败: 测试" {
		t.Fatalf("got %q", got)
	}
}
