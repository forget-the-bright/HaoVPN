package persist

// InsertAuditLog 追加一条审计记录到 audit_logs。
//
// 参数：e — Action/TargetType 等由调用方填好；ActorUserID、TargetID 可 nil。
// 返回：err 为 INSERT 失败（外键、磁盘等）。
// 副作用：写库；不可更新或删除（DeleteUser 仅置空 actor_user_id）。
func (s *Store) InsertAuditLog(e AuditEntry) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(actor_user_id, action, target_type, target_id, client_ip, detail_json) VALUES(?,?,?,?,?,?)`,
		e.ActorUserID, e.Action, e.TargetType, e.TargetID, e.ClientIP, e.DetailJSON)
	return err
}

// ListAuditLogs 分页查询审计日志，按 id 降序（最新在前）。
//
// 参数：limit — 每页条数；offset — 跳过条数（管理 API 分页）。
// 返回：[]AuditEntry；err 为查询失败。
// 副作用：只读。
func (s *Store) ListAuditLogs(limit, offset int) ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}
