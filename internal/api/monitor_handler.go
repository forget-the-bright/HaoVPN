package api

import (
	"net/http"
	"strconv"
	"time"

	"haovpn/internal/netutil"
	"haovpn/internal/readmodel"
)

// handleCSRF 返回当前会话的 CSRF Token（GET /api/v1/csrf）。
//
// 未登录或会话无效时返回 401；供 WebUI 写操作前刷新 token。
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session")
	if err != nil {
		writeAPIError(w, http.StatusUnauthorized, "未登录")
		return
	}
	token := s.auth.GetCSRF(c.Value)
	if token == "" {
		writeAPIError(w, http.StatusUnauthorized, "会话无效")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}

// handleMonitorOnline 返回当前在线 VPN 账号快照（GET /api/v1/monitor/online）。
//
// 每项含 DB 会话统计与内存 live 流量（Rx/Tx、allowed_ips）。
func (s *Server) handleMonitorOnline(w http.ResponseWriter, r *http.Request) {
	var items []map[string]any
	for _, uid := range s.sessions.ListOnline() {
		if item := s.buildAccountMonitorItemFromRow(uid, true); item != nil {
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleMonitorAccounts 返回 VPN 账号监控摘要（GET /api/v1/monitor/accounts）。
//
// 单次 JOIN 查询避免 N+1；支持 ?online=1 与 ?q= 用户名筛选。
func (s *Server) handleMonitorAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	onlineOnly := q.Get("online") == "1" || q.Get("online") == "true"
	nameQ := q.Get("q")

	rows, err := s.store.ListMonitorAccountRows()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	online := map[int64]bool{}
	for _, id := range s.sessions.ListOnline() {
		online[id] = true
	}

	var items []map[string]any
	for _, row := range rows {
		isOnline := online[row.ID]
		if onlineOnly && !isOnline {
			continue
		}
		if nameQ != "" && !containsFold(row.Username, nameQ) {
			continue
		}
		item := readmodel.MonitorRowToItem(row, isOnline)
		if isOnline {
			s.mergeLiveMonitorStats(item, row.ID)
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleMonitorEvents 分页返回连接事件（GET /api/v1/monitor/events）。
//
// 查询参数：user_id、event_type、limit、offset。
func (s *Server) handleMonitorEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clampLimit(parseIntDefault(q.Get("limit"), 50), 50, 200)
	offset := parseIntDefault(q.Get("offset"), 0)
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)

	items, total, err := s.store.ListConnectionEventsFiltered(readmodel.ConnectionEventFilter{
		UserID: userID, EventType: q.Get("event_type"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type eventView struct {
		ID         int64  `json:"id"`
		UserID     int64  `json:"user_id"`
		Username   string `json:"username"`
		EventType  string `json:"event_type"`
		RemoteAddr string `json:"remote_addr"`
		DetailJSON string `json:"detail_json,omitempty"`
		CreatedAt  string `json:"created_at"`
	}
	var out []eventView
	for _, e := range items {
		out = append(out, eventView{
			ID: e.ID, UserID: e.UserID, Username: s.store.UsernameByID(e.UserID),
			EventType: e.EventType, RemoteAddr: e.RemoteAddr, DetailJSON: e.DetailJSON,
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": out, "total": total, "limit": limit, "offset": offset,
	})
}

// buildAccountMonitorItemFromRow 将 userID 转为监控 API 单项 map。
//
// 合并 DB session_stats 与内存会话 live 统计；用户不存在时返回 nil。
func (s *Server) buildAccountMonitorItemFromRow(userID int64, online bool) map[string]any {
	u, err := s.store.GetUserByID(userID)
	if err != nil {
		return nil
	}
	st, _ := s.store.GetSessionStat(userID)
	row := readmodel.MonitorAccountRow{
		ID: u.ID, Username: u.Username, VPNIP: u.VPNIP, IPMode: u.IPMode,
		PolicyVer: u.PolicyVer, AllowedIPs: u.AllowedIPs,
	}
	if st != nil {
		row.ConnectedAt = st.ConnectedAt
		row.LastHeartbeat = st.LastHeartbeat
		row.RxBytes = st.RxBytes
		row.TxBytes = st.TxBytes
		row.ReconnectCount = st.ReconnectCount
		row.RemoteAddr = st.RemoteAddr
	}
	item := readmodel.MonitorRowToItem(row, online)
	if online {
		s.mergeLiveMonitorStats(item, userID)
	}
	return item
}

// mergeLiveMonitorStats 将内存会话的 Rx/Tx/AllowedIPs 合并进监控 item（在线时）。
func (s *Server) mergeLiveMonitorStats(item map[string]any, userID int64) {
	sess, ok := s.sessions.GetSession(userID)
	if !ok {
		return
	}
	readmodel.MergeLiveSessionStats(item, sess.RxBytes.Load(), sess.TxBytes.Load(), netutil.IPNetsToStrings(sess.AllowedIPs))
}

// containsFold 判断 s 是否包含 sub（ASCII 大小写不敏感）。
func containsFold(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && (s == sub || len(sub) > 0 && foldContains(s, sub))
}

// foldContains 在 s 中查找 sub 的首次出现（ASCII 大小写不敏感）。
func foldContains(s, sub string) bool {
	// 简单大小写不敏感包含
	return indexFold(s, sub) >= 0
}

// indexFold 返回 sub 在 s 中的起始下标；未找到返回 -1。
func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// equalFoldASCII 比较两字符串是否 ASCII 大小写不敏感相等。
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

