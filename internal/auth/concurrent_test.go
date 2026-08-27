package auth_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// TestConcurrentLoginFailuresLock 并发错误登录应累计失败次数并最终锁定（服务端 API 登录限流）。
func TestConcurrentLoginFailuresLock(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "auth-conc.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := auth.New(store, 5, 60, 3600)
	if err := svc.EnsureAdmin("admin", "SecurePass123!", false); err != nil {
		t.Fatal(err)
	}

	const workers = 10
	const attempts = 3 // 10*3=30 >= 5 次失败
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < attempts; j++ {
				_, _, _ = svc.Login("admin", "wrong-password", "203.0.113.50")
			}
		}()
	}
	wg.Wait()

	_, _, err = svc.Login("admin", "SecurePass123!", "203.0.113.50")
	if err == nil {
		t.Fatal("并发错密后应锁定，正确密码也不应通过")
	}
	if !strings.Contains(err.Error(), "稍后再试") {
		t.Fatalf("锁定错误信息: %v", err)
	}
}
