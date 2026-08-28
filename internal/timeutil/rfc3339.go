package timeutil

import (
	"strings"
	"time"
)

// FormatRFC3339 将 time.Time 格式化为 RFC3339 字符串（API JSON 统一出口）。
//
// 参数：t — 任意时区；内部转 UTC 再格式化，与 Web/API 历史行为一致。
func FormatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// FormatRFC3339Ptr 格式化可空时间；nil 或零值返回空串。
func FormatRFC3339Ptr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return FormatRFC3339(*t)
}

// ParseSinceRFC3339 解析 ?since= 等 RFC3339 查询参数。
//
// 参数：s — 空串或非法格式均返回零值 time.Time（表示不限制起始时间）。
func ParseSinceRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
