package persist_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestConcurrentCreateUsers SQLite WAL 下并发创建不同用户应全部成功。
func TestConcurrentCreateUsers(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "conc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const n = 30
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hash, err := auth.HashPassword("Password12345!")
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			_, err = store.CreateUser(fmt.Sprintf("user_%d", idx), hash, false)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("并发建用户失败 %d 次: %v", len(errs), errs[0])
	}
	cnt, err := store.CountUsers()
	if err != nil {
		t.Fatal(err)
	}
	if cnt != n {
		t.Fatalf("期望 %d 用户，实际 %d", n, cnt)
	}
}
