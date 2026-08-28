package persist

import (
	"database/sql"
	"time"

	"haovpn/internal/timeutil"
)

// SQLiteDateTime 兼容旧引用；新代码请直接用 timeutil.SQLiteDateTime。
const SQLiteDateTime = timeutil.SQLiteDateTime

// formatSQLiteTime 将 time.Time 格式化为 SQLite 文本列（UTC）；委托 timeutil。
func formatSQLiteTime(t time.Time) string {
	return timeutil.FormatUTC(t)
}

// parseSQLiteTime 解析 SQLite 日期时间文本；失败返回零值。
func parseSQLiteTime(s string) time.Time {
	return timeutil.ParseUTC(s)
}

// parseSQLiteTimePtr 解析可空 SQLite 时间列；无效或空返回 nil。
func parseSQLiteTimePtr(ns sql.NullString) *time.Time {
	return timeutil.ParseUTCPtr(ns)
}
