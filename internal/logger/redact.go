package logger

import (
	"regexp"
	"strings"
)

var (
	rePassword = regexp.MustCompile(`(?i)(password|passwd|secret|token|private_key)\s*[:=]\s*\S+`)
	reKey      = regexp.MustCompile(`(?i)[A-Za-z0-9+/]{40,}={0,2}`)
)

// needsRedact 快速判断行是否可能含敏感字段，避免每条日志跑重型正则。
func needsRedact(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "passwd") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "private_key") ||
		len(s) > 200
}

// RedactSensitive 脱敏日志行中的密码、token 与疑似密钥材料。
//
// 用于写盘/控制台前；无敏感关键词时原样返回。
// security.Redact 故意委托本函数（而非反向依赖），以打断 logger↔security 循环；
// 新代码优先直接调本函数。
func RedactSensitive(s string) string {
	if !needsRedact(s) {
		return s
	}
	s = rePassword.ReplaceAllString(s, "$1=[REDACTED]")
	if len(s) > 200 {
		s = reKey.ReplaceAllString(s, "[REDACTED_KEY]")
	}
	return s
}
