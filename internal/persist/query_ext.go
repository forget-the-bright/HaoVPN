package persist

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// userListCols Web 列表所需列（不含 password_hash / private_key_enc）。
const userListCols = `id, username, enabled, public_key, vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver`

// UserListItem Web 账号列表项（轻量）。
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

// UserListFilter 账号列表筛选。
type UserListFilter struct {
	Q          string
	Enabled    int  // 0 禁用 / 1 启用（须配合 UseEnabled）
	UseEnabled bool // true 时按 Enabled 筛选
	Limit      int
	Offset     int
}

// AuditListFilter 审计列表筛选。
type AuditListFilter struct {
	Action string
	Since  time.Time
	Limit  int
	Offset int
}

// ConnectionEventFilter 连接事件筛选。
type ConnectionEventFilter struct {
	UserID    int64
	EventType string
	Limit     int
	Offset    int
}

// MonitorAccountRow 监控页一行（用户 + session_stats JOIN）。
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

// ListUsersPage 分页列出账号（轻量列 + 可选筛选）。
func (s *Store) ListUsersPage(f UserListFilter) ([]UserListItem, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where := []string{"1=1"}
	args := []any{}
	if q := strings.TrimSpace(f.Q); q != "" {
		where = append(where, "INSTR(LOWER(username), LOWER(?)) > 0")
		args = append(args, q)
	}
	if f.UseEnabled {
		if f.Enabled == 1 {
			where = append(where, "enabled=1")
		} else {
			where = append(where, "enabled=0")
		}
	}
	wsql := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE `+wsql, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	qargs := append(append([]any{}, args...), f.Limit, f.Offset)
	rows, err := s.db.Query(`SELECT `+userListCols+` FROM users WHERE `+wsql+` ORDER BY id LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []UserListItem
	for rows.Next() {
		item, err := scanUserListItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func scanUserListItem(row scannable) (UserListItem, error) {
	var item UserListItem
	var enabled int
	var pubKey, vpnIP, ipsJSON, ipMode sql.NullString
	var ipLease, policyVer int
	if err := row.Scan(&item.ID, &item.Username, &enabled, &pubKey, &vpnIP, &ipsJSON, &ipMode, &ipLease, &policyVer); err != nil {
		return item, err
	}
	item.Enabled = enabled == 1
	item.HasVPN = pubKey.Valid && pubKey.String != ""
	item.VPNIP = vpnIP.String
	item.IPMode = ipMode.String
	item.PolicyVer = policyVer
	if ipsJSON.Valid && ipsJSON.String != "" {
		_ = json.Unmarshal([]byte(ipsJSON.String), &item.AllowedIPs)
	}
	return item, nil
}

// ListAuditLogsFiltered 带筛选的审计分页。
func (s *Store) ListAuditLogsFiltered(f AuditListFilter) ([]AuditEntry, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	where := []string{"1=1"}
	args := []any{}
	if a := strings.TrimSpace(f.Action); a != "" {
		where = append(where, "action=?")
		args = append(args, a)
	}
	if !f.Since.IsZero() {
		where = append(where, "created_at>=?")
		args = append(args, f.Since.UTC().Format("2006-01-02 15:04:05"))
	}
	wsql := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE `+wsql, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	qargs := append(append([]any{}, args...), f.Limit, f.Offset)
	rows, err := s.db.Query(`SELECT id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at
		FROM audit_logs WHERE `+wsql+` ORDER BY id DESC LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanAuditRows(rows)
	return out, total, err
}

func scanAuditRows(rows *sql.Rows) ([]AuditEntry, error) {
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var actor sql.NullInt64
		var target sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &actor, &e.Action, &e.TargetType, &target, &e.ClientIP, &e.DetailJSON, &created); err != nil {
			return nil, err
		}
		if actor.Valid {
			v := actor.Int64
			e.ActorUserID = &v
		}
		if target.Valid {
			v := target.Int64
			e.TargetID = &v
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListConnectionEventsFiltered 连接事件分页筛选。
func (s *Store) ListConnectionEventsFiltered(f ConnectionEventFilter) ([]ConnectionEvent, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	where := []string{"1=1"}
	args := []any{}
	if f.UserID > 0 {
		where = append(where, "user_id=?")
		args = append(args, f.UserID)
	}
	if et := strings.TrimSpace(f.EventType); et != "" {
		where = append(where, "event_type=?")
		args = append(args, et)
	}
	wsql := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM connection_events WHERE `+wsql, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	qargs := append(append([]any{}, args...), f.Limit, f.Offset)
	rows, err := s.db.Query(`SELECT id, user_id, event_type, remote_addr, detail_json, created_at
		FROM connection_events WHERE `+wsql+` ORDER BY id DESC LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []ConnectionEvent
	for rows.Next() {
		var e ConnectionEvent
		var created string
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.RemoteAddr, &e.DetailJSON, &created); err != nil {
			return nil, 0, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ListMonitorAccountRows 一次 JOIN 拉取监控所需字段，避免 N+1。
func (s *Store) ListMonitorAccountRows() ([]MonitorAccountRow, error) {
	rows, err := s.db.Query(`SELECT u.id, u.username, u.vpn_ip, u.ip_mode, u.policy_ver, u.allowed_ips,
		ss.connected_at, ss.last_heartbeat, ss.rx_bytes, ss.tx_bytes, ss.reconnect_count, ss.remote_addr
		FROM users u
		LEFT JOIN session_stats ss ON ss.user_id = u.id
		WHERE u.public_key IS NOT NULL AND u.public_key != ''
		ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MonitorAccountRow
	for rows.Next() {
		var r MonitorAccountRow
		var vpnIP, ipMode, ipsJSON sql.NullString
		var policyVer int
		var connAt, hb, remote sql.NullString
		var rx, tx sql.NullInt64
		var recon sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Username, &vpnIP, &ipMode, &policyVer, &ipsJSON,
			&connAt, &hb, &rx, &tx, &recon, &remote); err != nil {
			return nil, err
		}
		r.VPNIP = vpnIP.String
		r.IPMode = ipMode.String
		r.PolicyVer = policyVer
		if ipsJSON.Valid && ipsJSON.String != "" {
			_ = json.Unmarshal([]byte(ipsJSON.String), &r.AllowedIPs)
		}
		r.ConnectedAt = parseTimePtr(connAt)
		r.LastHeartbeat = parseTimePtr(hb)
		if rx.Valid {
			r.RxBytes = rx.Int64
		}
		if tx.Valid {
			r.TxBytes = tx.Int64
		}
		if recon.Valid {
			r.ReconnectCount = int(recon.Int64)
		}
		r.RemoteAddr = remote.String
		out = append(out, r)
	}
	return out, rows.Err()
}

func parseTimePtr(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", ns.String)
	if err != nil {
		return nil
	}
	return &t
}

// PruneAuditLogs 删除早于 cutoff 的审计记录。
func (s *Store) PruneAuditLogs(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneConnectionEvents 删除早于 cutoff 的连接事件。
func (s *Store) PruneConnectionEvents(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM connection_events WHERE created_at < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UsernameByID 按 ID 取用户名（事件列表展示用）。
func (s *Store) UsernameByID(id int64) string {
	var name string
	if err := s.db.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		return fmt.Sprintf("#%d", id)
	}
	return name
}
