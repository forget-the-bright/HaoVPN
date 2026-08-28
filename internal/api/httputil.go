package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"haovpn/internal/netutil"
	"haovpn/internal/timeutil"
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

// writeAPIError 返回标准 JSON 错误响应 {"error": msg}。
func writeAPIError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeOK 返回标准成功响应 {"ok": true}（HTTP 200）。
//
// 用途：踢线/删除/启禁/登出/改密等无额外载荷的写操作；禁止再手写 writeJSON(..., {"ok":true})。
func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writePage 返回标准分页 JSON 信封（items、total、limit、offset）。
//
// 用途：users/audit/monitor events 列表；history logs 因额外 source/lines/file 字段不套用本助手。
func writePage(w http.ResponseWriter, code int, items any, total, limit, offset int) {
	writeJSON(w, code, map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

// writeAttachment 以附件形式写出二进制/文本响应体。
//
// 参数：contentType — 如 application/zip；filename — Content-Disposition 文件名；body — 响应体。
// 用途：账号导出 ZIP/YAML，避免各 handler 重复写 Header。
func writeAttachment(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// parseSinceQuery 解析 ?since= RFC3339 时间过滤参数。
//
// 无效或缺失时返回零值 time.Time（表示不限制起始时间）。
func parseSinceQuery(r *http.Request) time.Time {
	return timeutil.ParseSinceRFC3339(r.URL.Query().Get("since"))
}

// clientIP 从 HTTP 请求提取客户端 IP 字符串。
//
// 优先 X-Forwarded-For 首段（反代场景，TrimSpace）；否则用 netutil.HostFromAddr
// 解析 RemoteAddr，正确处理 IPv6「[addr]:port」（禁止 LastIndex(":") 截断）。
// 关联：隧道侧来源 IP 亦用 HostFromAddr（tunnel/server_handler）。
func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	return netutil.HostFromAddr(r.RemoteAddr)
}
