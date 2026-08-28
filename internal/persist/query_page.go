package persist

import "database/sql"

// queryPageTotal 执行 COUNT + 分页 SELECT 的通用辅助，消除各 query_*.go 重复样板。
//
// 参数：countSQL/selectSQL 须含 WHERE 子句占位；args 为 WHERE 绑定参数；limit/offset 追加在末尾。
// 返回：total 为筛选后总行数；scan 对 SELECT 每行调用；err 为 COUNT/Query/Scan 失败。
func (s *Store) queryPageTotal(countSQL, selectSQL string, args []any, limit, offset int, scan func(*sql.Rows) error) (int, error) {
	var total int
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return 0, err
	}
	qargs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.Query(selectSQL, qargs...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return 0, err
		}
	}
	return total, rows.Err()
}
