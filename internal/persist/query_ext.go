package persist

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
)

// userListCols Web 列表所需列（不含 password_hash / private_key_enc）。
const userListCols = `id, username, enabled, public_key, vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver`

// ListUsersPage 分页列出账号（轻量列 + 可选筛选）。
//
// 参数：f — Limit/Offset 经 paginate 裁剪；Q 模糊匹配 username；UseEnabled+Enabled 筛选启用状态。
// 返回：items 不含 password_hash/private_key_enc；total 为筛选后总行数；err 为 COUNT 或 SELECT 失败。
// 副作用：只读 users 表。
// 并发：可与其他 Store 读操作并行；SQLite 层串行执行。
func (s *Store) ListUsersPage(f readmodel.UserListFilter) ([]readmodel.UserListItem, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	f.Offset = paginate.ClampOffset(f.Offset)

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

	var out []readmodel.UserListItem
	for rows.Next() {
		item, err := scanUserListItem(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func scanUserListItem(row scannable) (readmodel.UserListItem, error) {
	var item readmodel.UserListItem
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
	unmarshalAllowedIPs(ipsJSON, &item.AllowedIPs)
	return item, nil
}

// ListAuditLogsFiltered 带筛选的审计分页。
//
// 参数：f — Action 精确匹配；Since 过滤 created_at≥该时间；Limit/Offset 分页。
// 返回：[]AuditEntry 按 id 降序；total 为筛选后总数；err 为查询失败。
// 副作用：只读 audit_logs 表。
// 并发：可并行调用；只读无锁。
func (s *Store) ListAuditLogsFiltered(f readmodel.AuditListFilter) ([]AuditEntry, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	where := []string{"1=1"}
	args := []any{}
	if a := strings.TrimSpace(f.Action); a != "" {
		where = append(where, "action=?")
		args = append(args, a)
	}
	if !f.Since.IsZero() {
		where = append(where, "created_at>=?")
		args = append(args, formatSQLiteTime(f.Since))
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

// ListConnectionEventsFiltered 连接事件分页筛选。
//
// 参数：f — UserID>0 时按用户过滤；EventType 非空时精确匹配；Limit/Offset 分页。
// 返回：[]ConnectionEvent 按 id 降序；total 为筛选后总数；err 为查询失败。
// 副作用：只读 connection_events 表。
// 并发：可并行调用；只读无锁。
func (s *Store) ListConnectionEventsFiltered(f readmodel.ConnectionEventFilter) ([]ConnectionEvent, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
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

	out, err := scanConnectionEventRows(rows)
	return out, total, err
}

// ListMonitorAccountRows 一次 JOIN 拉取监控所需字段，避免 N+1。
//
// 返回：已配置公钥的 VPN 账号 + 左连接 session_stats（无会话时统计字段为零值）；
// err 为 JOIN 查询或 Scan 失败。
// 副作用：只读 users 与 session_stats 表。
// 并发：可并行调用；管理监控页刷新时使用。
func (s *Store) ListMonitorAccountRows() ([]readmodel.MonitorAccountRow, error) {
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

	var out []readmodel.MonitorAccountRow
	for rows.Next() {
		var r readmodel.MonitorAccountRow
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
		unmarshalAllowedIPs(ipsJSON, &r.AllowedIPs)
		r.ConnectedAt = parseSQLiteTimePtr(connAt)
		r.LastHeartbeat = parseSQLiteTimePtr(hb)
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

// PruneAuditLogs 删除早于 cutoff 的审计记录。
//
// 参数：cutoff — created_at 早于此 UTC 时间的行将被 DELETE。
// 返回：n 为实际删除行数；err 为执行失败（库已关闭、磁盘错误等）。
// 副作用：写 audit_logs 表；不可恢复，由 maintenance 定时任务调用。
// 并发：与其它写操作由 SQLite 串行化。
func (s *Store) PruneAuditLogs(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, formatSQLiteTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneConnectionEvents 删除早于 cutoff 的连接事件。
//
// 参数：cutoff — created_at 早于此 UTC 时间的行将被 DELETE。
// 返回：n 为实际删除行数；err 为执行失败。
// 副作用：写 connection_events 表；不可恢复，由 maintenance 定时任务调用。
// 并发：与其它写操作由 SQLite 串行化。
func (s *Store) PruneConnectionEvents(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM connection_events WHERE created_at < ?`, formatSQLiteTime(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UsernameByID 按 ID 取用户名（事件列表展示用）。
//
// 参数：id — users.id；不存在时返回 "#<id>" 占位字符串。
// 返回：username 或占位；永不返回 error（查询失败时降级为占位）。
// 副作用：只读 users 表一行。
// 并发：可并行调用；只读无锁。
func (s *Store) UsernameByID(id int64) string {
	var name string
	if err := s.db.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		return fmt.Sprintf("#%d", id)
	}
	return name
}
