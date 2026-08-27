package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"haovpn/internal/paginate"
)

// writeJSON 以 JSON 写入 HTTP 响应。
//
// 参数：code — HTTP 状态码；v — 可 JSON 序列化的响应体。
// 副作用：设置 Content-Type 并 WriteHeader。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIntDefault 解析整数字符串，失败或空时返回默认值（委托 paginate.ParseIntDefault）。
func parseIntDefault(s string, def int) int {
	return paginate.ParseIntDefault(s, def)
}

// clampLimit 限制 API 分页参数在 [def, max] 范围内（委托 paginate.ClampLimit）。
func clampLimit(n, def, max int) int {
	return paginate.ClampLimit(n, def, max)
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

// parseSinceQuery 解析 ?since= RFC3339 时间过滤参数。
//
// 无效或缺失时返回零值 time.Time（表示不限制起始时间）。
func parseSinceQuery(r *http.Request) time.Time {
	s := strings.TrimSpace(r.URL.Query().Get("since"))
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// clientIP 从 HTTP 请求提取客户端 IP 字符串。
//
// 优先 X-Forwarded-For 首段（反代场景）；否则解析 RemoteAddr 主机部分。
func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.Split(x, ",")[0]
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}
