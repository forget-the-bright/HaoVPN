package safeutil

import (
	"fmt"
	"time"
)

// RetryN 对 fn 最多尝试 attempts 次；失败则按 delay 休眠后重试。
//
// 参数：
//   attempts — ≤0 时按 1 次；
//   delay — 重试间隔；≤0 则不等待；
//   fn — 返回 nil 表示成功。
// 用途：管理面 Listen 等 TUN 就绪、短时资源竞争；不适合协议心跳/指数退避重连（用 transport.ReconnectClient）。
// 返回：最后一次错误；attempts 次皆失败时带中文包装。
func RetryN(attempts int, delay time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		last = fn()
		if last == nil {
			return nil
		}
		if i+1 < attempts && delay > 0 {
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("重试 %d 次仍失败: %w", attempts, last)
}
