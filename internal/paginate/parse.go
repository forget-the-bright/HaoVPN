package paginate

import "strconv"

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
