package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/paginate"
	"haovpn/internal/persist"
	"haovpn/internal/probedefense"
	"haovpn/internal/timeutil"
	"haovpn/internal/vpnaccount"
)

// writeJSON 以 JSON 写入 HTTP 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeJSONBody 解析请求 JSON 体到 dst；失败时写 400 并返回 false。
//
// 用途：peers/security/users 等写接口统一入口，避免各 handler 重复 NewDecoder+错误文案。
// 请求体上限 1MiB（与 decodeJSONOrForm 一致），防止管理面超大 JSON 内存 DoS。
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "无法读取请求体")
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		writeAPIError(w, http.StatusBadRequest, "无效 JSON")
		return false
	}
	return true
}

// requireMethod 校验 HTTP 方法是否在允许列表中；不匹配时写 405 并返回 false。
//
// 参数：methods — 如 http.MethodPost、http.MethodGet；至少一个。
// 用途：收敛各 handler 散落的 if r.Method != ... 样板。
func requireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	writeMethodNotAllowed(w)
	return false
}

// parseFormOrError 解析表单；失败写 400「invalid form data」并返回 false。
func parseFormOrError(w http.ResponseWriter, r *http.Request) bool {
	if err := parseRequestForm(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "表单数据无效")
		return false
	}
	return true
}

// decodeJSONOrForm 先尝试 JSON 解码到 dst；失败则 parseRequestForm 并调用 formFill 从 Form 填字段。
//
// 参数：
//   dst — JSON 目标结构体指针；
//   formFill — JSON 失败后从 r.FormValue 填充 dst（调用方闭包）；formFill 为 nil 时仅尝试 JSON。
// 返回：成功 true；JSON 与表单皆不可用时写 400。
// 为何：peers 写接口同时接受 JSON 与表单；须走 parseRequestForm（multipart），禁止裸 ParseForm。
func decodeJSONOrForm(w http.ResponseWriter, r *http.Request, dst any, formFill func()) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "无法读取请求体")
		return false
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed != "" && (trimmed[0] == '{' || trimmed[0] == '[') {
		if err := json.Unmarshal(body, dst); err == nil {
			return true
		}
	}
	// 重建 Body 供 ParseForm（已读尽）；multipart 走 parseRequestForm
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if formFill == nil {
		writeAPIError(w, http.StatusBadRequest, "无效 JSON")
		return false
	}
	if err := parseRequestForm(r); err != nil {
		writeAPIError(w, http.StatusBadRequest, "表单数据无效")
		return false
	}
	formFill()
	return true
}

// parsePathID 解析路径中的正整数 ID；失败写 400 并返回 (0, false)。
func parsePathID(w http.ResponseWriter, s string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || id <= 0 {
		writeAPIError(w, http.StatusBadRequest, "无效 ID")
		return 0, false
	}
	return id, true
}

// writeMethodNotAllowed 返回标准 405 JSON 错误。
func writeMethodNotAllowed(w http.ResponseWriter) {
	writeAPIError(w, http.StatusMethodNotAllowed, "方法不允许")
}

// writeAPIError 返回标准 JSON 错误响应 {"error": msg}。
func writeAPIError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeInternalError 记录服务端详因，对客户端仅返回稳定「内部错误」（防路径/SQL 泄漏）。
//
// 参数：err — 可为 nil（仍返回 500）；调用方已决定属 5xx。
// 关联：管理 API 原 writeAPIError(..., err.Error()) 全部收敛至此。
func writeInternalError(w http.ResponseWriter, err error) {
	if err != nil {
		logger.Error("api_internal: %v", err)
	} else {
		logger.Error("api_internal: unknown")
	}
	writeAPIError(w, http.StatusInternalServerError, "内部错误")
}

// writeOK 返回标准成功响应 {"ok": true}（HTTP 200）。
func writeOK(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// writeOKWith 返回 {"ok": true} 并合并 extra 字段（同名键覆盖 ok）。
//
// 用于带附加数据的成功响应（如 kicked、allow_all_vpn_peers），避免各 handler 手写 map。
func writeOKWith(w http.ResponseWriter, extra map[string]any) {
	out := map[string]any{"ok": true}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

// writePendingApply 返回 {"ok": true, "pending_apply": true} 并合并 extra。
//
// 托管路由/互访变更只写库不踢线时使用；控制台据此显示「须应用生效」黄条。
func writePendingApply(w http.ResponseWriter, extra map[string]any) {
	out := map[string]any{"ok": true, "pending_apply": true}
	for k, v := range extra {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

// writePage 返回标准分页 JSON 信封（items、total、limit、offset）。
func writePage(w http.ResponseWriter, code int, items any, total, limit, offset int) {
	writeJSON(w, code, map[string]any{
		"items": items, "total": total, "limit": limit, "offset": offset,
	})
}

// pageSliceBounds 计算内存列表分页的起止下标（offset 已夹到 ≥0）。
func pageSliceBounds(total, limit, offset int) (start, end int) {
	offset = paginate.ClampOffset(offset)
	if offset >= total {
		return 0, 0
	}
	end = offset + limit
	if end > total {
		end = total
	}
	return offset, end
}

// slicePage 对任意切片做 limit/offset 分页（空结果返回非 nil 空切片）。
func slicePage[T any](items []T, limit, offset int) []T {
	start, end := pageSliceBounds(len(items), limit, offset)
	if start >= end {
		return []T{}
	}
	return items[start:end]
}

// writeItems 返回仅含 items 的列表信封（无分页元数据）。
//
// 用途：peer-routes / peer-access / lan-registry / monitor/online 等全量或过滤列表。
func writeItems(w http.ResponseWriter, items any) {
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// writeItemsTotal 返回 items + total（无 limit/offset，适合内存过滤后的列表）。
func writeItemsTotal(w http.ResponseWriter, items any, total int) {
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// parseFormInt64 解析表单字段为 int64；空或非法时返回 0（调用方自行判 ≤0）。
func parseFormInt64(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue(key)), 10, 64)
	return v
}

// parseQueryInt64 解析 URL 查询参数为 int64；空或非法时返回 0。
func parseQueryInt64(r *http.Request, key string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(key)), 10, 64)
	return v
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

// writeAccountNotFound 账号不存在时统一 404。
func writeAccountNotFound(w http.ResponseWriter, err error) bool {
	if errors.Is(err, vpnaccount.ErrAccountNotFound) {
		writeAPIError(w, http.StatusNotFound, vpnaccount.ErrAccountNotFound.Error())
		return true
	}
	return false
}

// writeDomainError 将 persist/vpnaccount/auth/probedefense 等领域错误映射为 HTTP 状态。
//
// 固定 UX 文案（未登录/CSRF 等）仍由调用方 writeAPIError 字面量；本函数只处理 errors.Is 哨兵。
// 未知错误走 writeInternalError（防泄漏内部详因）。
func writeDomainError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if writeAccountNotFound(w, err) {
		return
	}
	switch {
	case errors.Is(err, vpnaccount.ErrLastAdmin),
		errors.Is(err, vpnaccount.ErrViaUserRequired),
		errors.Is(err, vpnaccount.ErrViaUserNotFound),
		errors.Is(err, vpnaccount.ErrViaNotVPN),
		errors.Is(err, vpnaccount.ErrPeerAccessArgs),
		errors.Is(err, persist.ErrInvalidPeerAccessPair),
		errors.Is(err, persist.ErrPeerAccessNotVPN),
		errors.Is(err, persist.ErrPeerAccessUserNotFound),
		errors.Is(err, auth.ErrWrongOldPassword),
		errors.Is(err, probedefense.ErrBanExempt),
		errors.Is(err, probedefense.ErrInvalidBanIP):
		writeAPIError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, vpnaccount.ErrPeerRouteNotFound):
		writeAPIError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, probedefense.ErrProbeGuardNotReady):
		writeAPIError(w, http.StatusServiceUnavailable, err.Error())
	default:
		// 校验类中文错误（密码强度、CIDR、用户名等）常见为非哨兵 fmt.Errorf，对客户端仍 400。
		msg := err.Error()
		if msg != "" && !strings.Contains(msg, "sql:") && !strings.Contains(strings.ToLower(msg), "sqlite") {
			writeAPIError(w, http.StatusBadRequest, msg)
			return
		}
		writeInternalError(w, err)
	}
}

// parsePathSuffixIP 从 URL Path 提取后缀 IP/CIDR 并校验（blocks/exempts DELETE）。
//
// allowCIDR true 时允许 CIDR；prefix 如 "/api/v1/security/blocks/"。
func parsePathSuffixIP(w http.ResponseWriter, r *http.Request, prefix string, allowCIDR bool) (string, bool) {
	ip := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if ip == "" {
		writeAPIError(w, http.StatusBadRequest, "无效 IP")
		return "", false
	}
	if err := netutil.ValidateIPOrCIDR("ip", ip, allowCIDR); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return ip, true
}
