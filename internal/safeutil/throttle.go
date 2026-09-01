package safeutil

import (
	"sync/atomic"
	"time"
)

// AllowEvery 日志/埋点限频：距上次放行不足 d 则 false，否则 CAS 更新时间戳并 true。
//
// 参数：last — 存放上次放行 UnixNano 的 atomic；nil 时恒 true（与旧 shouldWarn*(nil) 一致）。
//
//	d — 最小间隔；<=0 时每次 true（仍会写 last）。
//
// 用途：伪造源/越权目的 WARN、send queue full、tun_upload_quiesced 等；**非**业务配额或安全门禁。
// 并发：多 goroutine 对同一 last 安全；CAS 失败视为本轮不放行（另一侧已打日志）。
func AllowEvery(last *atomic.Int64, d time.Duration) bool {
	if last == nil {
		return true
	}
	now := time.Now().UnixNano()
	prev := last.Load()
	if d > 0 && prev != 0 && now-prev < int64(d) {
		return false
	}
	return last.CompareAndSwap(prev, now)
}
