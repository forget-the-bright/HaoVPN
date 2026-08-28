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
