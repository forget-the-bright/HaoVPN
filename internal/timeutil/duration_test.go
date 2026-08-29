package timeutil

import (
	"testing"
	"time"
)

// TestSeconds 确认整数秒映射为 Duration，含 0。
func TestSeconds(t *testing.T) {
	if got := Seconds(5); got != 5*time.Second {
		t.Fatalf("Seconds(5)=%v", got)
	}
	if got := Seconds(0); got != 0 {
		t.Fatalf("Seconds(0)=%v", got)
	}
}
