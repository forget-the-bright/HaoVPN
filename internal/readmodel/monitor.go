package readmodel

import (
	"haovpn/internal/timeutil"
)

// MonitorItem 监控 API 返回的账号摘要（map 键与 WebUI 一致）。
type MonitorItem map[string]any

// MonitorRowToItem 将 DB JOIN 行转为监控 JSON 项。
func MonitorRowToItem(row MonitorAccountRow, online bool) MonitorItem {
	item := MonitorItem{
		"user_id": row.ID, "username": row.Username, "vpn_ip": row.VPNIP,
		"ip_mode": row.IPMode, "policy_ver": row.PolicyVer,
		"allowed_ips": row.AllowedIPs, "online": online,
		"rx_bytes": row.RxBytes, "tx_bytes": row.TxBytes,
		"reconnect_count": row.ReconnectCount, "remote_addr": row.RemoteAddr,
	}
	if row.ConnectedAt != nil {
		item["connected_at"] = timeutil.FormatRFC3339Ptr(row.ConnectedAt)
	}
	if row.LastHeartbeat != nil {
		item["last_heartbeat"] = timeutil.FormatRFC3339Ptr(row.LastHeartbeat)
	}
	return item
}

// MergeLiveSessionStats 用在线会话覆盖流量与 allowed_ips（会话内实时值优先）。
func MergeLiveSessionStats(item MonitorItem, rx, tx int64, allowedIPs []string) {
	item["rx_bytes"] = rx
	item["tx_bytes"] = tx
	if len(allowedIPs) > 0 {
		item["allowed_ips"] = allowedIPs
	}
}

// UserListAccountView Web 账号列表中的 accountView 形状。
type UserListAccountView struct {
	ID         int64    `json:"id"`
	Username   string   `json:"username"`
	Enabled    bool     `json:"enabled"`
	HasVPN     bool     `json:"has_vpn"`
	VPNIP      string   `json:"vpn_ip,omitempty"`
	IPMode     string   `json:"ip_mode,omitempty"`
	PolicyVer  int      `json:"policy_ver,omitempty"`
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	Online     bool     `json:"online"`
}

// UserListItemToAccountView 将轻量列表项转为 accountView。
func UserListItemToAccountView(u UserListItem, online bool) UserListAccountView {
	return UserListAccountView{
		ID: u.ID, Username: u.Username, Enabled: u.Enabled,
		HasVPN: u.HasVPN, VPNIP: u.VPNIP, IPMode: u.IPMode,
		PolicyVer: u.PolicyVer, AllowedIPs: u.AllowedIPs, Online: online,
	}
}
