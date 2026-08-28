package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/timeutil"
)

// writeJSON 以 JSON 写入 HTTP 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeMethodNotAllowed 返回标准 405 JSON 错误。
func writeMethodNotAllowed(w http.ResponseWriter) {
	writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
}

// writeAPIError 返回标准 JSON 错误响应 {"error": msg}。
func writeAPIError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeOK 返回标准成功响应 {"ok": true}（HTTP 200）。
func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writePage 返回标准分页 JSON 信封（items、total、limit、offset）。
func writePage(w http.ResponseWriter, code int, items any, total, limit, offset int) {
	writeJSON(w, code, map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

// writeAttachment 以附件形式写出二进制/文本响应体。
func writeAttachment(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// parseSinceQuery 解析 ?since= RFC3339 时间过滤参数。
func parseSinceQuery(r *http.Request) time.Time {
	return timeutil.ParseSinceRFC3339(r.URL.Query().Get("since"))
}

// resolveClientIP 从 HTTP 请求提取客户端 IP。
//
// trustedProxyCIDRs 为空时仅使用 RemoteAddr（防 XFF 轮换绕过登录锁定）。
// 仅当 RemoteAddr 命中 trusted_proxy_cidrs 时才解析 X-Forwarded-For 首跳。
func resolveClientIP(r *http.Request, trustedProxyCIDRs []string) string {
	remoteIP := netutil.HostFromAddr(r.RemoteAddr)
	if len(trustedProxyCIDRs) > 0 {
		if ip := net.ParseIP(remoteIP); ip != nil && netutil.IPMatchesRules(ip, trustedProxyCIDRs) {
			if x := r.Header.Get("X-Forwarded-For"); x != "" {
				return strings.TrimSpace(strings.Split(x, ",")[0])
			}
		}
	}
	return remoteIP
}

// clientIP 使用 Server 配置的 trusted_proxy_cidrs 解析客户端 IP。
func (s *Server) clientIP(r *http.Request) string {
	return resolveClientIP(r, s.cfg.API.TrustedProxyCIDRs)
}

// redactLogLines 对 API 返回的日志行脱敏。
func redactLogLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = logger.RedactSensitive(line)
	}
	return out
}
