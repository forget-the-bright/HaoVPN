package logger

import (
	"regexp"
	"strings"
)

var (
	// rePassword 匹配 password/passwd/secret/token/private_key/csrf_token 等 key=value（不含泛化 session，避免误伤业务日志）。
	rePassword = regexp.MustCompile(`(?i)(password|passwd|secret|token|private_key|csrf_token)\s*[:=]\s*\S+`)
	// reSessionToken 仅匹配 session= / session: 形式的会话值，避免把 host_id 等长 hex 当 token。
	reSessionToken = regexp.MustCompile(`(?i)(session)\s*[:=]\s*[A-Za-z0-9+/=_-]{16,}`)
	// reBearer 匹配 Authorization: Bearer <token> / Basic …
	reBearer = regexp.MustCompile(`(?i)(authorization)\s*[:=]\s*(bearer|basic)\s+\S+`)
	reKey    = regexp.MustCompile(`(?i)[A-Za-z0-9+/]{40,}={0,2}`)
)

// needsRedact 快速判断行是否可能含敏感字段，避免每条日志跑重型正则。
func needsRedact(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "password") ||
		strings.Contains(lower, "passwd") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "private_key") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "csrf") ||
		strings.Contains(lower, "session=") ||
		strings.Contains(lower, "session:") ||
		len(s) > 200
}

// RedactSensitive 脱敏日志行中的密码、token、Authorization 与疑似密钥材料。
//
// 用于写盘/控制台前；无敏感关键词时原样返回。
// security.Redact 故意委托本函数（而非反向依赖），以打断 logger↔security 循环；
// 新代码优先直接调本函数。
// 注意：不对「任意长 hex」做全局替换，以免误伤 host_id / 公钥指纹等运维字段。
func RedactSensitive(s string) string {
	if !needsRedact(s) {
		return s
	}
	s = rePassword.ReplaceAllString(s, "$1=[REDACTED]")
	s = reSessionToken.ReplaceAllString(s, "$1=[REDACTED]")
	s = reBearer.ReplaceAllString(s, "$1=[REDACTED]")
	if len(s) > 200 {
		s = reKey.ReplaceAllString(s, "[REDACTED_KEY]")
	}
	return s
}
