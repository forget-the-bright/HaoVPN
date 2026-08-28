package persist

import (
	"database/sql"
	"strings"
	"time"

	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
	"haovpn/internal/timeutil"
)

// ListConnectionEventsFiltered 连接事件分页筛选（JOIN users 取 username，避免 N+1）。
//
// 参数：f — UserID>0 时按用户过滤；EventType 非空时精确匹配；Limit/Offset 分页。
// 返回：[]readmodel.ConnectionEventRow 按 id 降序；total 为筛选后总数。
func (s *Store) ListConnectionEventsFiltered(f readmodel.ConnectionEventFilter) ([]readmodel.ConnectionEventRow, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	where := []string{"1=1"}
	args := []any{}
	if f.UserID > 0 {
		where = append(where, "ce.user_id=?")
		args = append(args, f.UserID)
	}
	if et := strings.TrimSpace(f.EventType); et != "" {
		where = append(where, "ce.event_type=?")
		args = append(args, et)
	}
	wsql := strings.Join(where, " AND ")
	var out []readmodel.ConnectionEventRow
	total, err := s.queryPageTotal(
		`SELECT COUNT(*) FROM connection_events ce WHERE `+wsql,
		`SELECT ce.id, ce.user_id, COALESCE(u.username,''), ce.event_type, ce.remote_addr, ce.detail_json, ce.created_at
		FROM connection_events ce
		LEFT JOIN users u ON u.id = ce.user_id
		WHERE `+wsql+` ORDER BY ce.id DESC LIMIT ? OFFSET ?`,
		args, f.Limit, f.Offset,
		func(rows *sql.Rows) error {
			var r readmodel.ConnectionEventRow
			var created string
			if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.EventType, &r.RemoteAddr, &r.DetailJSON, &created); err != nil {
				return err
			}
			r.CreatedAt = timeutil.ParseUTC(created)
			out = append(out, r)
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// PruneConnectionEvents 删除早于 cutoff 的连接事件。
//
// 参数：cutoff — created_at 早于此 UTC 时间的行将被 DELETE。
// 返回：n 为实际删除行数；err 为执行失败。
// 副作用：写 connection_events 表；不可恢复，由 maintenance 定时任务调用。
// 并发：与其它写操作由 SQLite 串行化。
func (s *Store) PruneConnectionEvents(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM connection_events WHERE created_at < ?`, timeutil.FormatUTC(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
