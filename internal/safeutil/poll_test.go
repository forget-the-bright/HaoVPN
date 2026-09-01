package safeutil

import (
	"testing"
	"time"
)

// TestPollUntilSuccess 条件很快满足时须返回 true。
func TestPollUntilSuccess(t *testing.T) {
	n := 0
	ok := PollUntil(time.Now().Add(time.Second), 10*time.Millisecond, nil, func() bool {
		n++
		return n >= 2
	})
	if !ok {
		t.Fatal("应在条件满足时返回 true")
	}
}

// TestPollUntilAbort 中途 abort 须尽快返回 false，不得拖满 deadline。
func TestPollUntilAbort(t *testing.T) {
	start := time.Now()
	aborted := false
	ok := PollUntil(time.Now().Add(3*time.Second), 200*time.Millisecond, func() bool {
		return time.Since(start) > 80*time.Millisecond
	}, func() bool {
		aborted = true
		return false
	})
	if ok {
		t.Fatal("abort 时应返回 false")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("abort 后应尽快返回，elapsed=%s", time.Since(start))
	}
	_ = aborted
}

// TestPollUntilTimeout 永不满足时到期返回 false。
func TestPollUntilTimeout(t *testing.T) {
	start := time.Now()
	ok := PollUntil(time.Now().Add(120*time.Millisecond), 30*time.Millisecond, nil, func() bool {
		return false
	})
	if ok {
		t.Fatal("超时应返回 false")
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("应接近 deadline 才返回")
	}
}
