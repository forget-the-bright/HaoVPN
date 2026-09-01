package sessionmgr

import (
	"testing"
	"time"
)

// TestShouldWarnSpoof 伪造源 WARN 限流：与越权目的对称。
func TestShouldWarnSpoof(t *testing.T) {
	ps := &AccountSession{UserID: 1}
	if !shouldWarnSpoof(ps) {
		t.Fatal("first should warn")
	}
	if shouldWarnSpoof(ps) {
		t.Fatal("within 10s must rate-limit")
	}
	ps.lastSpoofWarn.Store(time.Now().Add(-11 * time.Second).UnixNano())
	if !shouldWarnSpoof(ps) {
		t.Fatal("after interval should warn again")
	}
	if !shouldWarnSpoof(nil) {
		t.Fatal("nil ps should warn")
	}
}

// TestShouldWarnDstOverreach 越权目的 WARN 限流：首条 true，窗口内 false，过期后再 true。
func TestShouldWarnDstOverreach(t *testing.T) {
	ps := &AccountSession{UserID: 1}
	if !shouldWarnDstOverreach(ps) {
		t.Fatal("first should warn")
	}
	if shouldWarnDstOverreach(ps) {
		t.Fatal("within 10s must rate-limit")
	}
	ps.lastDstWarn.Store(time.Now().Add(-11 * time.Second).UnixNano())
	if !shouldWarnDstOverreach(ps) {
		t.Fatal("after interval should warn again")
	}
}

// TestShouldWarnDstOverreachNil 空会话仍允许 WARN（降级安全）。
func TestShouldWarnDstOverreachNil(t *testing.T) {
	if !shouldWarnDstOverreach(nil) {
		t.Fatal("nil ps should warn")
	}
}
