package persist

import (
	"database/sql"

	"haovpn/internal/timeutil"
)

// scanAuditRow 从 audit_logs 查询结果集扫描单行 AuditEntry。
func scanAuditRow(rows *sql.Rows) (AuditEntry, error) {
	var e AuditEntry
	var actor sql.NullInt64
	var target sql.NullInt64
	var created string
	if err := rows.Scan(&e.ID, &actor, &e.Action, &e.TargetType, &target, &e.ClientIP, &e.DetailJSON, &created); err != nil {
		return e, err
	}
	if actor.Valid {
		v := actor.Int64
		e.ActorUserID = &v
	}
	if target.Valid {
		v := target.Int64
		e.TargetID = &v
	}
	e.CreatedAt = timeutil.ParseUTC(created)
	return e, nil
}

// scanAuditRows 从 audit_logs 查询结果集扫描多行 AuditEntry。
//
// 参数：rows — 须为 SELECT id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at。
// 返回：按扫描顺序的切片；Scan 失败时立即返回 err。
func scanAuditRows(rows *sql.Rows) ([]AuditEntry, error) {
	var out []AuditEntry
	for rows.Next() {
		e, err := scanAuditRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// scanConnectionEventRows 从 connection_events 结果集扫描多行 ConnectionEvent。
func scanConnectionEventRows(rows *sql.Rows) ([]ConnectionEvent, error) {
	var out []ConnectionEvent
	for rows.Next() {
		var e ConnectionEvent
		var created string
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.RemoteAddr, &e.DetailJSON, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = timeutil.ParseUTC(created)
		out = append(out, e)
	}
	return out, rows.Err()
}
