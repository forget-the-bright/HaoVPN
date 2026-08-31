package clientapp

import (
	"testing"
	"time"
)

// TestConnectedSinceSetAndClear 进入 Connected 记录时间；Stop 清零。
func TestConnectedSinceSetAndClear(t *testing.T) {
	eng := NewEngine(nil)
	if !eng.ConnectedSince().IsZero() {
		t.Fatal("初始应为零")
	}
	eng.mu.Lock()
	eng.state = StateConnected
	eng.connectedAt = time.Now()
	eng.mu.Unlock()
	if eng.ConnectedSince().IsZero() {
		t.Fatal("Connected 后应有时间")
	}
	eng.setState(StateIdle)
	if !eng.ConnectedSince().IsZero() {
		t.Fatal("Idle 后应清零")
	}
}
