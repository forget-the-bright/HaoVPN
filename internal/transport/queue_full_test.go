package transport

import (
	"testing"
	"time"
)

// TestNoteSendQueueFullRateLimit 同一 Conn 连续满队列时 drops 累加，但 WARN 限频不阻断返回。
func TestNoteSendQueueFullRateLimit(t *testing.T) {
	c := &Conn{cfg: Config{MaxQueueSize: 1}, sendQ: make(chan []byte, 1)}
	c.sendQ <- []byte{1} // 填满
	for i := 0; i < 3; i++ {
		err := c.SendRaw(FrameTypeData, []byte("x"))
		if err == nil || err.Error() != "send queue full" {
			t.Fatalf("i=%d err=%v", i, err)
		}
	}
	if c.queueFullDrops.Load() != 3 {
		t.Fatalf("drops=%d", c.queueFullDrops.Load())
	}
	// 限频窗口内再次调用仍只更新 drops
	c.noteSendQueueFull(FrameTypeData)
	if c.queueFullDrops.Load() != 4 {
		t.Fatalf("drops after note=%d", c.queueFullDrops.Load())
	}
	// 推进“上次 WARN”时间戳后允许再次打日志路径（不崩即可）
	c.lastQueueFullWarnUnixNano.Store(time.Now().Add(-6 * time.Second).UnixNano())
	c.noteSendQueueFull(FrameTypeData)
	if c.queueFullDrops.Load() != 5 {
		t.Fatalf("drops=%d", c.queueFullDrops.Load())
	}
}
