package netutil

import "strings"

// StringSlicesEqualTrimmed 比较两个字符串切片：长度相同且逐元素 TrimSpace 后相等（顺序敏感）。
//
// 用途：DNS 服务器列表差分（clientapp / netstack），避免空格差异导致重复 apply。
func StringSlicesEqualTrimmed(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}
