package paginate

import (
	"net/url"
	"strconv"
	"strings"
)

// ParseOnlyEnabled 解析安全列表 GET 的「仅生效项」过滤（enabled!=0 且 all!=1）。
func ParseOnlyEnabled(q url.Values) bool {
	return q.Get("enabled") != "0" && q.Get("all") != "1"
}

// ParseBoolQuery 解析 HTTP 查询布尔参数（1/true/0/false）。
//
// 参数：s — 如 ?online=1、?enabled=true；空串表示未传参。
// 返回：val 为解析结果；ok 为 true 表示调用方显式传入了可识别的布尔值。
func ParseBoolQuery(s string) (val, ok bool) {
	switch strings.TrimSpace(s) {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	default:
		return false, false
	}
}

// ParseIntDefault 解析整数字符串，失败或空时返回默认值。
//
// 参数：s — 查询参数等；def — 解析失败时的回退值。
// 返回：成功时为解析整数，否则 def。
func ParseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ParseLimitOffset 从 URL 查询参数解析分页 limit/offset。
//
// 参数：
//   q — 通常为 r.URL.Query()；读 limit、offset 键。
//   defLimit — 缺省或非法 limit 时的默认每页条数（传给 ClampLimit）。
//   maxLimit — 允许的最大每页条数。
//
// 返回：limit 已夹在 [defLimit, maxLimit]；offset 为解析整数（未传或非法则为 0，可为负，调用方若需非负可再 ClampOffset）。
// 用途：api 各列表接口统一解析，避免手写 ClampLimit+ParseIntDefault 三行样板。
func ParseLimitOffset(q url.Values, defLimit, maxLimit int) (limit, offset int) {
	limit = ClampLimit(ParseIntDefault(q.Get("limit"), defLimit), defLimit, maxLimit)
	offset = ParseIntDefault(q.Get("offset"), 0)
	return limit, offset
}
