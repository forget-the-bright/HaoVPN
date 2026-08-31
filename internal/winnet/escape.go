package winnet

import "strings"

// EscapeSingleQuoted 转义嵌入 PowerShell 单引号字符串字面量的内容。
//
// 参数：s — 待嵌入 '...' 的原始文本。
// 返回：将单引号替换为 '' 后的字符串。
// 跨平台：纯字符串，无系统调用；Windows 脚本与单测共用。
func EscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
