package api

import (
	"net/http"
	"strconv"

	"haovpn/internal/netutil"
	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
)

// handleMonitorOnline 返回当前在线 VPN 账号快照（GET /api/v1/monitor/online）。
//
// 复用 ListMonitorAccountRows JOIN 路径，避免逐用户 GetUserByID 的 N+1。
func (s *Server) handleMonitorOnline(w http.ResponseWriter, r *http.Request) {
	online := s.onlineUserSet()
	rows, err := s.store.ListMonitorAccountRows(readmodel.MonitorAccountFilter{})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := s.buildMonitorItems(rows, true, online)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleMonitorAccounts 返回 VPN 账号监控摘要（GET /api/v1/monitor/accounts）。
//
// 单次 JOIN 查询避免 N+1；支持 ?online=1 与 ?q= 用户名筛选（q 在 SQL 层过滤）。
func (s *Server) handleMonitorAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	onlineOnly, _ := paginate.ParseBoolQuery(q.Get("online"))

	rows, err := s.store.ListMonitorAccountRows(readmodel.MonitorAccountFilter{NameQuery: q.Get("q")})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	online := s.onlineUserSet()
	items := s.buildMonitorItems(rows, onlineOnly, online)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleMonitorEvents 分页返回连接事件（GET /api/v1/monitor/events）。
//
// 查询参数：user_id、event_type、limit、offset；username 由 SQL JOIN 一次带出。
func (s *Server) handleMonitorEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 200)
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)

	rows, total, err := s.store.ListConnectionEventsFiltered(readmodel.ConnectionEventFilter{
		UserID: userID, EventType: q.Get("event_type"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]readmodel.ConnectionEventView, len(rows))
	for i, row := range rows {
		out[i] = readmodel.ConnectionEventToView(row)
	}
	writePage(w, http.StatusOK, out, total, limit, offset)
}

// buildMonitorItems 将 JOIN 行转为监控 JSON item，可选仅在线与合并 live 统计。
func (s *Server) buildMonitorItems(rows []readmodel.MonitorAccountRow, onlineOnly bool, online map[int64]bool) []map[string]any {
	var items []map[string]any
	for _, row := range rows {
		isOnline := online[row.ID]
		if onlineOnly && !isOnline {
			continue
		}
		item := readmodel.MonitorRowToItem(row, isOnline)
		if isOnline {
			s.mergeLiveMonitorStats(item, row.ID)
		}
		items = append(items, item)
	}
	return items
}

// onlineUserSet 将会话管理器在线列表转为 map，便于监控页 O(1) 判定。
func (s *Server) onlineUserSet() map[int64]bool {
	online := map[int64]bool{}
	for _, id := range s.sessions.ListOnline() {
		online[id] = true
	}
	return online
}

// mergeLiveMonitorStats 将内存会话的 Rx/Tx/AllowedIPs 合并进监控 item（在线时）。
func (s *Server) mergeLiveMonitorStats(item map[string]any, userID int64) {
	sess, ok := s.sessions.GetSession(userID)
	if !ok {
		return
	}
	readmodel.MergeLiveSessionStats(item, sess.RxBytes.Load(), sess.TxBytes.Load(), netutil.IPNetsToStrings(sess.AllowedIPs))
}
