package safeutil_test

import (
	"errors"
	"testing"
	"time"

	"haovpn/internal/safeutil"
)

func TestRetryNSucceedsOnSecond(t *testing.T) {
	n := 0
	err := safeutil.RetryN(3, time.Millisecond, func() error {
		n++
		if n < 2 {
			return errors.New("暂不可用")
		}
		return nil
	})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestRetryNExhausts(t *testing.T) {
	err := safeutil.RetryN(2, 0, func() error { return errors.New("fail") })
	if err == nil {
		t.Fatal("应失败")
	}
}

func TestExpBackoff(t *testing.T) {
	if got := safeutil.ExpBackoff(time.Second, 8*time.Second); got != 2*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := safeutil.ExpBackoff(8*time.Second, 8*time.Second); got != 8*time.Second {
		t.Fatalf("cap: got %v", got)
	}
	if got := safeutil.ExpBackoff(0, 5*time.Second); got != 5*time.Second {
		t.Fatalf("zero current: got %v", got)
	}
}

