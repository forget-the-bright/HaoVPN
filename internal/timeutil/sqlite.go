package timeutil

import (
	"database/sql"
	"time"
)

// SQLiteDateTime 为 haovpn.db / logs.db 中 UTC 时间文本列的统一 layout。
//
// 为何固定此格式：SQLite 无原生 DATETIME 类型，项目约定存 UTC 字符串便于排序与跨库一致。
const SQLiteDateTime = "2006-01-02 15:04:05"

// FormatUTC 将 time.Time 格式化为 SQLite 文本列（强制 UTC）。
//
// 参数：t — 任意时区的时间；内部转 UTC 再格式化。
// 返回：layout 对应的字符串，永不为空（零值时间为 0001-01-01 00:00:00）。
func FormatUTC(t time.Time) string {
	return t.UTC().Format(SQLiteDateTime)
}

// ParseUTC 解析 SQLite 日期时间文本；失败返回零值 time.Time（不返回 error，便于扫描行）。
//
// 参数：s — 列文本；空串或非法格式均得零值。
func ParseUTC(s string) time.Time {
	t, _ := time.Parse(SQLiteDateTime, s)
	return t
}

// ParseUTCPtr 解析可空 SQLite 时间列；无效或空返回 nil。
//
// 参数：ns — sql.NullString；Valid=false 或 String 空/非法时返回 nil。
func ParseUTCPtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(SQLiteDateTime, ns.String)
	if err != nil {
		return nil
	}
	return &t
}
