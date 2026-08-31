package netutil

import "strings"

// TrimLower 去掉首尾空白后转小写。
//
// 用途：查询参数、CIDR 比较键等对大小写不敏感的规范化（api 审计 source 过滤、
// clientgui 托盘路由去重键）。与 DedupTrimNonEmpty 互补：本函数处理单字符串。
//
// 参数：s — 任意输入；空串或仅空白返回 ""。
func TrimLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
