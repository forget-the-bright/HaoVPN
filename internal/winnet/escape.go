package winnet

import (
	"regexp"
	"strings"
)

// EscapeSingleQuoted 转义嵌入 PowerShell 单引号字符串字面量的内容。
//
// 参数：s — 待嵌入 '...' 的原始文本。
// 返回：将单引号替换为 '' 后的字符串。
// 跨平台：纯字符串，无系统调用；Windows 脚本与单测共用。
//
// 转义方言（勿与其它层混用）：
//   - EscapeSingleQuoted：PowerShell '...' 字面量（本函数）；
//   - EscapeRegex：PowerShell/NET -match 正则字面量（先于本函数使用）；
//   - platform.EscapeArg：Windows 命令行 argv；
//   - autostart shellQuote/xmlEscape：desktop/systemd/plist。
func EscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// EscapeRegex 将字符串转义为可嵌入 PowerShell -match 的字面量（.NET 正则）。
//
// 为何需要：-match 把右侧当正则；仅 EscapeSingleQuoted 挡不住 . * | ( ) 等元字符，
// 恶意或误配的 tun.name / 描述可能扩大匹配面（误删网卡）。
//
// 参数：s — 希望按字面匹配的文本。
// 返回：regexp.QuoteMeta 结果（与 .NET 常用元字符转义兼容）。
// 嵌入顺序：先 EscapeRegex，再 EscapeSingleQuoted，最后放进 '...'。
func EscapeRegex(s string) string {
	return regexp.QuoteMeta(s)
}
