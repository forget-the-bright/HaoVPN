package logger

import (
	"sync"
)

const defaultRecentCap = 50

// RecentRecorder 环形缓冲最近 WARN/ERROR 摘要（供 health/Dashboard）。
type RecentRecorder struct {
	mu    sync.RWMutex
	lines []string
	cap   int
}

var globalRecent = NewRecentRecorder(defaultRecentCap)

// NewRecentRecorder 创建指定容量的最近错误记录器。
func NewRecentRecorder(cap int) *RecentRecorder {
	if cap <= 0 {
		cap = defaultRecentCap
	}
	return &RecentRecorder{cap: cap}
}

// Add 追加一条摘要（超出容量丢弃最旧）。
func (r *RecentRecorder) Add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	if len(r.lines) > r.cap {
		r.lines = r.lines[len(r.lines)-r.cap:]
	}
}

// Snapshot 返回当前缓冲副本。
func (r *RecentRecorder) Snapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// RecordRecent 向全局缓冲追加 WARN/ERROR 摘要。
func RecordRecent(line string) {
	globalRecent.Add(line)
}

// RecentErrors 返回全局最近 WARN/ERROR 摘要。
func RecentErrors() []string {
	return globalRecent.Snapshot()
}
