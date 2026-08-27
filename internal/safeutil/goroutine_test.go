package safeutil_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
)

func init() {
	_ = logger.Init(logger.Config{Level: "error"})
}

func TestGoSafeRecoversPanic(t *testing.T) {
	done := make(chan struct{})
	safeutil.GoSafe("panic-test", func() {
		defer close(done)
		panic("test panic")
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not finish after panic")
	}
}

func TestShutdownWait(t *testing.T) {
	sd := safeutil.NewShutdown()
	var count atomic.Int32
	sd.Go("worker", func(ctx context.Context) {
		count.Add(1)
		<-ctx.Done()
	})
	time.Sleep(50 * time.Millisecond)
	sd.Cancel()
	sd.Wait(2 * time.Second)
	if count.Load() != 1 {
		t.Fatalf("expected worker to run")
	}
}
