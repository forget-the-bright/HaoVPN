package safeutil

import "time"

// pollAbortSlice 可打断休眠的切片长度：醒后检查 abort，避免长 tick 卡死取消路径。
const pollAbortSlice = 50 * time.Millisecond

// PollUntil 在 deadline 前周期性调用 fn，直到成功、abort 或超时。
//
// 参数：
//   deadline — 绝对截止时间（通常 time.Now().Add(timeout)）；
//   tick — 两次 fn 之间的休眠上限；≤0 时默认 50ms；
//   abort — 可选；返回 true 时立即停止并返回 false（HardRestart 退出登录/抢占）；
//   fn — 轮询体；返回 true 表示条件已满足。
//
// 返回：fn 曾为 true；abort 或超时则为 false。
//
// 为何集中：WaitDNSReadyAbort、GUI 单实例等待、Windows 服务停等曾各自手写
// for time.Now().Before + Sleep，易漏 abort 切片。休眠按 pollAbortSlice 切开以便响应 abort。
//
// 不适用于：带 SetReadDeadline/Peek 的 I/O 探测（如 transport banner）——语义不同，勿硬套。
func PollUntil(deadline time.Time, tick time.Duration, abort func() bool, fn func() bool) bool {
	if fn == nil {
		return false
	}
	if tick <= 0 {
		tick = pollAbortSlice
	}
	for {
		if abort != nil && abort() {
			return false
		}
		if fn() {
			return true
		}
		now := time.Now()
		if !now.Before(deadline) {
			return false
		}
		sleepUntil := now.Add(tick)
		if sleepUntil.After(deadline) {
			sleepUntil = deadline
		}
		for time.Now().Before(sleepUntil) {
			if abort != nil && abort() {
				return false
			}
			left := time.Until(sleepUntil)
			if left <= 0 {
				break
			}
			d := pollAbortSlice
			if left < d {
				d = left
			}
			time.Sleep(d)
		}
	}
}
