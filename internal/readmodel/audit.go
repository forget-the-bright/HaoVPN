package readmodel

import (
	"time"

	"haovpn/internal/timeutil"
)

// AuditLogView 管理审计 API 返回项（RFC3339 时间、与 WebUI 契约一致）。
type AuditLogView struct {
	ID          int64  `json:"id"`
	ActorUserID *int64 `json:"actor_user_id,omitempty"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type"`
	TargetID    *int64 `json:"target_id,omitempty"`
	ClientIP    string `json:"client_ip"`
	DetailJSON  string `json:"detail_json,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// AuditLogViewFrom 从 persist 扫描字段构建审计 API 视图。
func AuditLogViewFrom(id int64, actor *int64, action, targetType string, targetID *int64, clientIP, detailJSON string, createdAt time.Time) AuditLogView {
	return AuditLogView{
		ID: id, ActorUserID: actor, Action: action, TargetType: targetType,
		TargetID: targetID, ClientIP: clientIP, DetailJSON: detailJSON,
		CreatedAt: timeutil.FormatRFC3339(createdAt),
	}
}

// ConnectionEventRow 连接事件 JOIN users 后的读模型行。
type ConnectionEventRow struct {
	ID         int64
	UserID     int64
	Username   string
	EventType  string
	RemoteAddr string
	DetailJSON string
	CreatedAt  time.Time
}

// ConnectionEventView 连接事件 API 返回项。
type ConnectionEventView struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	EventType  string `json:"event_type"`
	RemoteAddr string `json:"remote_addr"`
	DetailJSON string `json:"detail_json,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// ConnectionEventToView 将 JOIN 行转为 API JSON 视图。
func ConnectionEventToView(row ConnectionEventRow) ConnectionEventView {
	return ConnectionEventView{
		ID: row.ID, UserID: row.UserID, Username: row.Username,
		EventType: row.EventType, RemoteAddr: row.RemoteAddr, DetailJSON: row.DetailJSON,
		CreatedAt: timeutil.FormatRFC3339(row.CreatedAt),
	}
}

// MonitorAccountFilter 监控账号列表 SQL 筛选（用户名子串）。
type MonitorAccountFilter struct {
	NameQuery string
}
