package api

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"haovpn/internal/persist"
)

// handleCSRF 返回当前会话的 CSRF Token。
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录"})
		return
	}
	token := s.auth.GetCSRF(c.Value)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "会话无效"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}

// handleMonitorOnline 返回当前在线账号。
func (s *Server) handleMonitorOnline(w http.ResponseWriter, r *http.Request) {
	var items []map[string]any
	for _, uid := range s.sessions.ListOnline() {
		if item := s.buildAccountMonitorItemFromRow(uid, true); item != nil {
			items = append(items, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleMonitorAccounts 返回 VPN 账号监控摘要（单次 JOIN，无 N+1）。
func (s *Server) handleMonitorAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	onlineOnly := q.Get("online") == "1" || q.Get("online") == "true"
	nameQ := q.Get("q")

	rows, err := s.store.ListMonitorAccountRows()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
		item := monitorRowToItem(row, isOnline)
		if isOnline {
			if sess, ok := s.sessions.GetSession(row.ID); ok {
				item["rx_bytes"] = sess.RxBytes.Load()
				item["tx_bytes"] = sess.TxBytes.Load()
				if len(sess.AllowedIPs) > 0 {
					item["allowed_ips"] = ipNetsToStrings(sess.AllowedIPs)
				}
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// handleMonitorEvents 返回最近连接事件（分页 + 筛选）。
func (s *Server) handleMonitorEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := clampLimit(parseIntDefault(q.Get("limit"), 50), 50, 200)
	offset := parseIntDefault(q.Get("offset"), 0)
	userID, _ := strconv.ParseInt(q.Get("user_id"), 10, 64)

	items, total, err := s.store.ListConnectionEventsFiltered(persist.ConnectionEventFilter{
		UserID: userID, EventType: q.Get("event_type"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
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

func monitorRowToItem(row persist.MonitorAccountRow, online bool) map[string]any {
	item := map[string]any{
		"user_id": row.ID, "username": row.Username, "vpn_ip": row.VPNIP,
		"ip_mode": row.IPMode, "policy_ver": row.PolicyVer,
		"allowed_ips": row.AllowedIPs, "online": online,
		"rx_bytes": row.RxBytes, "tx_bytes": row.TxBytes,
		"reconnect_count": row.ReconnectCount, "remote_addr": row.RemoteAddr,
	}
	if row.ConnectedAt != nil {
		item["connected_at"] = row.ConnectedAt.Format(time.RFC3339)
	}
	if row.LastHeartbeat != nil {
		item["last_heartbeat"] = row.LastHeartbeat.Format(time.RFC3339)
	}
	return item
}

func (s *Server) buildAccountMonitorItemFromRow(userID int64, online bool) map[string]any {
	u, err := s.store.GetUserByID(userID)
	if err != nil {
		return nil
	}
	st, _ := s.store.GetSessionStat(userID)
	row := persist.MonitorAccountRow{
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
	item := monitorRowToItem(row, online)
	if sess, ok := s.sessions.GetSession(userID); ok && online {
		item["rx_bytes"] = sess.RxBytes.Load()
		item["tx_bytes"] = sess.TxBytes.Load()
		if len(sess.AllowedIPs) > 0 {
			item["allowed_ips"] = ipNetsToStrings(sess.AllowedIPs)
		}
	}
	return item
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || len(s) >= len(sub) && (s == sub || len(sub) > 0 && foldContains(s, sub))
}

func foldContains(s, sub string) bool {
	// 简单大小写不敏感包含
	return indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFoldASCII(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

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

func ipNetsToStrings(nets []*net.IPNet) []string {
	var out []string
	for _, n := range nets {
		if n != nil {
			out = append(out, n.String())
		}
	}
	return out
}
