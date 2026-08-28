package health

import "time"

// DashboardMap 构造 Dashboard API 响应 map（与 handleHealth 字段对齐，含兼容键 online_peers）。
func DashboardMap(started time.Time, onlineCount int, dbOK, tunOK, natOK bool, recent []string) map[string]any {
	st := NewStatus(started, onlineCount, dbOK, tunOK, natOK, recent)
	return map[string]any{
		"online_accounts": onlineCount,
		"online_peers":    onlineCount,
		"uptime_sec":      st.UptimeSec,
		"db_ok":           st.DBOK,
		"tun_ok":          st.TunOK,
		"nat_ok":          st.NatOK,
		"recent_errors":   st.RecentErrors,
	}
}
