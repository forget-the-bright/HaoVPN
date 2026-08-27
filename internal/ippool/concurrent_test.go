package ippool_test

import (
	"sync"
	"testing"

	"haovpn/internal/ippool"
)

// TestConcurrentAllocateUnique 并发分配 IP 不得重复（服务端 peer 创建路径）。
func TestConcurrentAllocateUnique(t *testing.T) {
	p, err := ippool.New("10.88.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	p.Reserve("10.88.0.1")

	const n = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	ips := make(map[string]int64)
	errs := make([]error, 0)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(peerID int64) {
			defer wg.Done()
			ip, err := p.Allocate(peerID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if prev, dup := ips[ip]; dup {
				t.Errorf("重复 IP %s: peer %d 与 %d", ip, prev, peerID)
			}
			ips[ip] = peerID
		}(int64(i + 2))
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("allocate errors: %v", errs)
	}
	if len(ips) != n {
		t.Fatalf("期望 %d 个唯一 IP，实际 %d", n, len(ips))
	}
}
