package persist

import (
	"database/sql"
	"time"
)

// SQLiteDateTime 为 haovpn.db / logs.db 中 UTC 时间文本列的 layout。
const SQLiteDateTime = "2006-01-02 15:04:05"

// formatSQLiteTime 将 time.Time 格式化为 SQLite 文本列（UTC）。
func formatSQLiteTime(t time.Time) string {
	return t.UTC().Format(SQLiteDateTime)
}

// parseSQLiteTime 解析 SQLite 日期时间文本；失败返回零值 time.Time。
func parseSQLiteTime(s string) time.Time {
	t, _ := time.Parse(SQLiteDateTime, s)
	return t
}

// parseSQLiteTimePtr 解析可空 SQLite 时间列；无效或空返回 nil。
func parseSQLiteTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(SQLiteDateTime, ns.String)
	if err != nil {
		return nil
	}
	return &t
}
