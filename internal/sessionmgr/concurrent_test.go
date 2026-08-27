package sessionmgr_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"haovpn/internal/sessionmgr"
)

// TestConcurrentKickAndOnlineCount 并发踢线与读在线数不得 panic（服务端 sessionmgr 路径）。
func TestConcurrentKickAndOnlineCount(t *testing.T) {
	mgr := sessionmgr.New(nil)
	var kicks atomic.Int64

	mgr.SetKickHandler(func(peerID int64) {
		kicks.Add(1)
	})

	const rounds = 200
	var wg sync.WaitGroup
	for i := 0; i < rounds; i++ {
		wg.Add(2)
		go func(id int64) {
			defer wg.Done()
			mgr.KickUser(id)
		}(int64(i))
		go func() {
			defer wg.Done()
			_ = mgr.OnlineCount()
			_ = mgr.ListOnline()
		}()
	}
	wg.Wait()

	if kicks.Load() != rounds {
		t.Fatalf("KickHandler 调用次数=%d，期望 %d", kicks.Load(), rounds)
	}
}
