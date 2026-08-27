package safeutil

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"haovpn/internal/logger"
)

// GoSafe 在独立 goroutine 中执行 fn，panic 时 recover 并打 Error 日志。
//
// 参数：name 用于日志标识 goroutine；fn 不应再向外 panic（已被吞掉）。
// 返回：无；立即返回，不等待 fn 结束。
// 副作用：启动 goroutine；panic 不传播到调用方。
func GoSafe(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine %s panic: %v", name, r)
			}
		}()
		fn()
	}()
}

// GoSafeCtx 在 GoSafe 包装下运行 fn(ctx)，直至 fn 返回或 ctx 取消（fn 须自行监听 ctx）。
//
// 参数：ctx 传入 fn；name 同 GoSafe。
// 返回：无；异步执行。
// 副作用：同 GoSafe；不自动在 ctx 取消时中断 fn。
func GoSafeCtx(ctx context.Context, name string, fn func(context.Context)) {
	GoSafe(name, func() {
		fn(ctx)
	})
}

// Shutdown 协调 SIGINT/SIGTERM 与组件 goroutine 的优雅退出。
//
// 字段：
//   ctx / cancel — 根 context；Cancel 或信号 handler 触发 cancel。
//   wg — 跟踪 Shutdown.Go 注册的 goroutine。
//   once — 保证 Wait 只执行一次超时逻辑。
//   done — Wait 开始后置 true，Done() 可查。
//
// 线程安全：Go/Wait/Cancel 可来自不同 goroutine；Wait 应在 Cancel 后调用。
type Shutdown struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	done   atomic.Bool
}

// NewShutdown 创建监听 SIGINT/SIGTERM 的关闭协调器。
//
// 参数：无。
// 返回：已注册 signal.Notify 的 *Shutdown；收到信号时自动 cancel 根 ctx。
// 副作用：启动 signal-handler goroutine；进程生命周期内勿重复 New。
func NewShutdown() *Shutdown {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Shutdown{ctx: ctx, cancel: cancel}
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	GoSafe("signal-handler", func() {
		select {
		case sig := <-ch:
			logger.Info("received signal %s, initiating graceful shutdown", sig)
			s.cancel()
		case <-ctx.Done():
		}
		signal.Stop(ch)
	})
	return s
}

// Context 返回根 shutdown context，供组件 select Done 或传递子 ctx。
//
// 参数：无。
// 返回：Cancel 或 OS 信号触发后 Done 关闭的 context.Context。
// 副作用：无。
func (s *Shutdown) Context() context.Context { return s.ctx }

// Go 在受 wg 跟踪的 goroutine 中执行 fn，传入根 shutdown context。
//
// 参数：name 用于 panic 日志；fn 应在 ctx.Done 时尽快退出。
// 返回：无；wg.Add 在启动前完成。
// 副作用：启动 goroutine；Wait 会等待其 Done。
func (s *Shutdown) Go(name string, fn func(context.Context)) {
	s.wg.Add(1)
	GoSafe(name, func() {
		defer s.wg.Done()
		fn(s.ctx)
	})
}

// Cancel 主动触发 shutdown（等同收到终止信号）。
//
// 参数：无。
// 返回：无。
// 副作用：cancel 根 ctx；重复调用安全。
func (s *Shutdown) Cancel() { s.cancel() }

// Wait 阻塞直至所有 Shutdown.Go 的 goroutine 结束或超时。
//
// 参数：timeout 最大等待时长；到期打 Warn 但不强制杀 goroutine。
// 返回：无。
// 副作用：once 内置位 done；内部再起 goroutine 调 wg.Wait。
func (s *Shutdown) Wait(timeout time.Duration) {
	s.once.Do(func() {
		s.done.Store(true)
	})
	done := make(chan struct{})
	GoSafe("shutdown-wait", func() {
		s.wg.Wait()
		close(done)
	})
	select {
	case <-done:
		logger.Info("all goroutines stopped")
	case <-time.After(timeout):
		logger.Warn("shutdown timeout after %s, some goroutines may still be running", timeout)
	}
}

// Done 报告是否已调用 Wait（shutdown 流程已进入等待阶段）。
//
// 参数：无。
// 返回：Wait 首次执行后为 true；仅 Cancel 未 Wait 时为 false。
// 副作用：无；atomic 读。
func (s *Shutdown) Done() bool { return s.done.Load() }
