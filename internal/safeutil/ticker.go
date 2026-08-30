package safeutil

import (
	"context"
	"time"
)

// RunTicker 在 ctx 存活期间按 interval 周期调用 fn；ctx 取消后立即退出。
//
// 参数：
//   ctx — 取消时停止 ticker 循环。
//   interval — ticker 间隔；≤0 时直接返回（不启动）。
//   fn — 每次 tick 执行；应快速返回，长时间任务应另起 goroutine。
//
// 副作用：阻塞调用 goroutine 直至 ctx.Done()；内部 NewTicker 并在退出时 Stop。
// 并发：同一 ctx 上不应并行多次 RunTicker（通常单 goroutine 调用）。
func RunTicker(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

// RunTickerStop 在 stop 通道关闭前按 interval 周期调用 fn；收到 stop 后立即退出。
//
// 与 RunTicker 的区别：用 chan struct{} 而非 context，便于旧式 stop 信号集成。
// interval≤0：直接返回。fn 应短小；耗时工作请另起 GoSafe goroutine。
func RunTickerStop(stop <-chan struct{}, interval time.Duration, fn func()) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			fn()
		}
	}
}
