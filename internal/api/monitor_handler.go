package api

import (
	"net/http"

	"haovpn/internal/flowmon"
	"haovpn/internal/netutil"
	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
	"haovpn/internal/timeutil"
)

// handleMonitorOnline 返回当前在线 VPN 账号快照（GET /api/v1/monitor/online）。
//
// 支持 ?limit=&offset= 分页（总览页）；缺省全量兼容旧前端。
func (s *Server) handleMonitorOnline(w http.ResponseWriter, r *http.Request) {
	online := s.onlineUserSet()
	rows, err := s.store.ListMonitorAccountRows(readmodel.MonitorAccountFilter{})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	items := s.buildMonitorItems(rows, true, online)
	q := r.URL.Query()
	if q.Get("limit") != "" || q.Get("offset") != "" {
		limit, offset := paginate.ParseLimitOffset(q, 50, 200)
		total := len(items)
		page := sliceMonitorItems(items, limit, offset)
		writePage(w, http.StatusOK, page, total, limit, offset)
		return
	}
	writeItems(w, items)
}

// handleMonitorAccounts 返回 VPN 账号监控摘要（GET /api/v1/monitor/accounts）。
//
// 支持 ?online=1、?q=、?limit=&offset=；标准 writePage 信封。
func (s *Server) handleMonitorAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	onlineOnly, _ := paginate.ParseBoolQuery(q.Get("online"))
	limit, offset := paginate.ParseLimitOffset(q, 50, 200)

	rows, err := s.store.ListMonitorAccountRows(readmodel.MonitorAccountFilter{NameQuery: q.Get("q")})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	online := s.onlineUserSet()
	items := s.buildMonitorItems(rows, onlineOnly, online)
	total := len(items)
	page := sliceMonitorItems(items, limit, offset)
	writePage(w, http.StatusOK, page, total, limit, offset)
}

// handleMonitorEvents 分页返回连接事件（GET /api/v1/monitor/events）。
func (s *Server) handleMonitorEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 200)
	userID := parseQueryInt64(r, "user_id")

	rows, total, err := s.store.ListConnectionEventsFiltered(readmodel.ConnectionEventFilter{
		UserID: userID, EventType: q.Get("event_type"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}
	out := make([]readmodel.ConnectionEventView, len(rows))
	for i, row := range rows {
		out[i] = readmodel.ConnectionEventToView(row)
	}
	writePage(w, http.StatusOK, out, total, limit, offset)
}

// handleMonitorFlows 分页返回 L4 流表（GET /api/v1/monitor/flows）。
func (s *Server) handleMonitorFlows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := paginate.ParseLimitOffset(q, 50, 200)
	if s.sessions == nil || s.sessions.Flows == nil {
		// 无流表时仍回显请求的 limit/offset，便于前端分页控件一致
		writePage(w, http.StatusOK, []any{}, 0, limit, offset)
		return
	}
	userID := parseQueryInt64(r, "user_id")
	proto := paginate.ParseIntDefault(q.Get("proto"), 0)
	items, total := s.sessions.Flows.List(flowmon.ListFilter{
		UserID: userID, Proto: proto,
		SrcIP: q.Get("src_ip"), DstIP: q.Get("dst_ip"),
		Sort:   flowmon.ParseSortQuery(q.Get("sort")),
		Limit:  limit, Offset: offset,
	})
	byID, _ := s.userDirMap()
	out := make([]map[string]any, len(items))
	for i, f := range items {
		name := ""
		if u, ok := byID[f.UserID]; ok {
			name = u.Username
		}
		out[i] = map[string]any{
			"user_id": f.UserID, "username": name,
			"src_ip": f.SrcIP, "dst_ip": f.DstIP,
			"proto": f.Proto, "proto_name": f.ProtoName,
			"sport": f.Sport, "dport": f.Dport,
			"bytes_in": f.BytesIn, "bytes_out": f.BytesOut,
			"packets_in": f.PacketsIn, "packets_out": f.PacketsOut,
			"bytes_total": f.BytesTotal,
			"first_seen": timeutil.FormatRFC3339(f.FirstSeen),
			"last_seen":  timeutil.FormatRFC3339(f.LastSeen),
		}
	}
	writePage(w, http.StatusOK, out, total, limit, offset)
}

func sliceMonitorItems(items []map[string]any, limit, offset int) []map[string]any {
	offset = paginate.ClampOffset(offset)
	if offset >= len(items) {
		return []map[string]any{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
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

func (s *Server) onlineUserSet() map[int64]bool {
	online := map[int64]bool{}
	for _, id := range s.sessions.ListOnline() {
		online[id] = true
	}
	return online
}

func (s *Server) mergeLiveMonitorStats(item map[string]any, userID int64) {
	sess, ok := s.sessions.GetSession(userID)
	if !ok {
		return
	}
	readmodel.MergeLiveSessionStats(item, sess.RxBytes.Load(), sess.TxBytes.Load(), netutil.IPNetsToStrings(sess.AllowedIPs))
}
