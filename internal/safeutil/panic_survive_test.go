package safeutil_test

import (
	"sync/atomic"
	"testing"
	"time"

	"haovpn/internal/safeutil"
)

// TestGoSafeBusinessPathSurvivesPanic 模拟业务 goroutine panic 后进程内其它逻辑仍可继续（step11 #3）。
func TestGoSafeBusinessPathSurvivesPanic(t *testing.T) {
	var healthOK atomic.Bool
	healthOK.Store(true)

	done := make(chan struct{})
	safeutil.GoSafe("session-worker", func() {
		defer close(done)
		panic("simulated session panic")
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("panic goroutine did not finish")
	}

	// 另一条安全路径仍可运行
	alive := make(chan struct{})
	safeutil.GoSafe("health-probe", func() {
		if !healthOK.Load() {
			t.Error("health should still be ok")
		}
		close(alive)
	})
	select {
	case <-alive:
	case <-time.After(2 * time.Second):
		t.Fatal("health path did not run after panic")
	}
}
