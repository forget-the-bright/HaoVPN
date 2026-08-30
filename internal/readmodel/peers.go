package readmodel

// PeerRouteView 托管路由列表行（展示 dest via vpn_ip + 失效/成员）。
//
// 填充：api.toPeerRouteView；JSON 契约供 WebUI 托管路由表。
type PeerRouteView struct {
	ID            int64   `json:"id"`
	DestCIDR      string  `json:"dest_cidr"`
	ViaUserID     int64   `json:"via_user_id"`
	ViaUsername   string  `json:"via_username"`
	ViaVPNIP      string  `json:"via_vpn_ip,omitempty"`
	Display       string  `json:"display"`
	ViaOffline    bool    `json:"via_offline"`
	Stale         bool    `json:"stale"` // via 离线或注册表无匹配 dest
	Scope         string  `json:"scope"` // all | user
	MemberUserIDs []int64 `json:"member_user_ids"`
	MemberNames   string  `json:"member_names,omitempty"`
}

// PeerAccessView 互访白名单一行。
type PeerAccessView struct {
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	PeerUserID   int64  `json:"peer_user_id"`
	PeerUsername string `json:"peer_username"`
	PeerVPNIP    string `json:"peer_vpn_ip,omitempty"`
}

// LANRegistryView 客户端本地网段注册表一行。
type LANRegistryView struct {
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	DestCIDR  string `json:"dest_cidr"`
	VPNIP     string `json:"vpn_ip"`
	HostID    string `json:"host_id,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
