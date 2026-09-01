package safeutil

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestAllowEvery 首条放行、窗内拒绝、过期再放行；nil last 恒 true。
func TestAllowEvery(t *testing.T) {
	if !AllowEvery(nil, time.Second) {
		t.Fatal("nil last")
	}
	var last atomic.Int64
	if !AllowEvery(&last, 10*time.Second) {
		t.Fatal("first")
	}
	if AllowEvery(&last, 10*time.Second) {
		t.Fatal("within window")
	}
	last.Store(time.Now().Add(-11 * time.Second).UnixNano())
	if !AllowEvery(&last, 10*time.Second) {
		t.Fatal("after interval")
	}
}
