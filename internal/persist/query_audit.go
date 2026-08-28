package persist

import (
	"strings"
	"time"

	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
	"haovpn/internal/timeutil"
)

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
		args = append(args, timeutil.FormatUTC(f.Since))
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

// PruneAuditLogs 删除早于 cutoff 的审计记录。
//
// 参数：cutoff — created_at 早于此 UTC 时间的行将被 DELETE。
// 返回：n 为实际删除行数；err 为执行失败（库已关闭、磁盘错误等）。
// 副作用：写 audit_logs 表；不可恢复，由 maintenance 定时任务调用。
// 并发：与其它写操作由 SQLite 串行化。
func (s *Store) PruneAuditLogs(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, timeutil.FormatUTC(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
