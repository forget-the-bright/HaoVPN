package safeutil

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsCanceled 覆盖 Canceled / DeadlineExceeded / 普通错误 / nil。
func TestIsCanceled(t *testing.T) {
	if IsCanceled(nil) {
		t.Fatal("nil 不应为 canceled")
	}
	if IsCanceled(errors.New("x")) {
		t.Fatal("普通错误不应为 canceled")
	}
	if !IsCanceled(context.Canceled) {
		t.Fatal("Canceled 应为 true")
	}
	if !IsCanceled(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded 应为 true")
	}
	if !IsCanceled(errors.Join(errors.New("wrap"), context.Canceled)) {
		t.Fatal("wrapped Canceled 应为 true")
	}
}

// TestIsDeadline 仅 DeadlineExceeded；Canceled 必须为 false。
func TestIsDeadline(t *testing.T) {
	if IsDeadline(nil) {
		t.Fatal("nil")
	}
	if IsDeadline(context.Canceled) {
		t.Fatal("Canceled 不是 deadline")
	}
	if !IsDeadline(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded")
	}
	if !IsDeadline(errors.Join(errors.New("wrap"), context.DeadlineExceeded)) {
		t.Fatal("wrapped DeadlineExceeded")
	}
}

// TestCheck nil ctx 与已取消 / 未取消。
func TestCheck(t *testing.T) {
	if err := Check(nil); err != nil {
		t.Fatalf("nil ctx 应返回 nil，got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := Check(ctx); err != nil {
		t.Fatalf("未取消应 nil，got %v", err)
	}
	cancel()
	if err := Check(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消应 Canceled，got %v", err)
	}
	dctx, dcancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer dcancel()
	time.Sleep(2 * time.Millisecond)
	if err := Check(dctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("超时应 DeadlineExceeded，got %v", err)
	}
}
