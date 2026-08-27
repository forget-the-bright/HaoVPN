// Package readmodel 定义 Web/API 读模型 DTO，与 SQLite 存储层解耦。
//
// 上游：api 序列化 JSON；persist 查询结果 scan 填充。
// 下游：无 internal 业务包依赖（仅标准库）。
// 不变量：字段名与 JSON tag 与 WebUI 契约一致；SQL 逻辑留在 persist。
package readmodel

import "time"

// UserListItem Web 账号列表项（轻量，不含 password_hash / private_key_enc）。
type UserListItem struct {
	ID         int64    `json:"id"`
	Username   string   `json:"username"`
	Enabled    bool     `json:"enabled"`
	HasVPN     bool     `json:"has_vpn"`
	VPNIP      string   `json:"vpn_ip,omitempty"`
	IPMode     string   `json:"ip_mode,omitempty"`
	PolicyVer  int      `json:"policy_ver,omitempty"`
	AllowedIPs []string `json:"allowed_ips,omitempty"`
}

// UserListFilter 账号列表分页筛选条件。
type UserListFilter struct {
	Q          string
	Enabled    int
	UseEnabled bool
	Limit      int
	Offset     int
}

// AuditListFilter 审计日志分页筛选条件。
type AuditListFilter struct {
	Action string
	Since  time.Time
	Limit  int
	Offset int
}

// ConnectionEventFilter 连接事件分页筛选条件。
type ConnectionEventFilter struct {
	UserID    int64
	EventType string
	Limit     int
	Offset    int
}

// MonitorAccountRow 监控页一行（users JOIN session_stats）。
type MonitorAccountRow struct {
	ID             int64
	Username       string
	VPNIP          string
	IPMode         string
	PolicyVer      int
	AllowedIPs     []string
	ConnectedAt    *time.Time
	LastHeartbeat  *time.Time
	RxBytes        int64
	TxBytes        int64
	ReconnectCount int
	RemoteAddr     string
}
