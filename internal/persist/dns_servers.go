package persist

import (
	"database/sql"
	"fmt"
	"strings"

	"haovpn/internal/timeutil"
)

// dns_servers.go：托管 DNS（dns_servers / members / excludes）CRUD 与按账号解析。

// CreateDNSServer 新增手工托管 DNS；members 规范化后写入；excludes 可选。
//
// 参数：dnsIP/remark — 调用方原始输入；memberIDs 须非空（含 0=全部）或由上层 ApplyAll 传入 [0]；
//
//	空成员一律拒绝（禁止静默变成全部，与托管路由一致）；excludeIDs 可空。
//
// 返回：新行 id；IP 冲突或校验失败返回中文 error。
func (s *Store) CreateDNSServer(dnsIP, remark string, memberIDs, excludeIDs []int64) (int64, error) {
	ip, err := ValidateDNSIP(dnsIP)
	if err != nil {
		return 0, err
	}
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return 0, fmt.Errorf("包含集不能为空")
	}
	excludes := NormalizeExcludeUserIDs(excludeIDs)
	// 指定成员时清空排除（语义上多余）
	if !PeerRouteHasAllMembers(members) {
		excludes = nil
	}
	if err := s.validateDNSMemberIDs(members); err != nil {
		return 0, err
	}
	if err := s.validateDNSExcludeIDs(excludes); err != nil {
		return 0, err
	}
	remark = strings.TrimSpace(remark)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO dns_servers(dns_ip, remark, source) VALUES(?,?,?)`,
		ip, remark, DNSSourceManual,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("相同 DNS IP 已存在")
		}
		return 0, fmt.Errorf("插入 dns_servers: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := insertDNSMembersTx(tx, id, members); err != nil {
		return 0, err
	}
	if err := insertDNSExcludesTx(tx, id, excludes); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateDNSServerRemark 更新备注（config / manual 均可）。
func (s *Store) UpdateDNSServerRemark(id int64, remark string) error {
	remark = strings.TrimSpace(remark)
	res, err := s.db.Exec(
		`UPDATE dns_servers SET remark=?, updated_at=datetime('now') WHERE id=?`,
		remark, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("托管 DNS 不存在")
	}
	return nil
}

// ReplaceDNSServerMembers 替换包含集（仅 manual；config 禁止调用方应在上层拦截）。
func (s *Store) ReplaceDNSServerMembers(id int64, memberIDs []int64) error {
	rt, err := s.GetDNSServer(id)
	if err != nil {
		return err
	}
	if rt == nil {
		return fmt.Errorf("托管 DNS 不存在")
	}
	if rt.IsConfigSource() {
		return fmt.Errorf("配置文件 DNS 的包含集固定为全部账号，不可修改")
	}
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return fmt.Errorf("包含集不能为空")
	}
	if err := s.validateDNSMemberIDs(members); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dns_server_members WHERE dns_id=?`, id); err != nil {
		return err
	}
	if err := insertDNSMembersTx(tx, id, members); err != nil {
		return err
	}
	// 改为指定账号时清空排除
	if !PeerRouteHasAllMembers(members) {
		if _, err := tx.Exec(`DELETE FROM dns_server_excludes WHERE dns_id=?`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE dns_servers SET updated_at=datetime('now') WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplaceDNSServerExcludes 替换排除集（仅当包含集为 all 时有意义）。
func (s *Store) ReplaceDNSServerExcludes(id int64, excludeIDs []int64) error {
	rt, err := s.GetDNSServer(id)
	if err != nil {
		return err
	}
	if rt == nil {
		return fmt.Errorf("托管 DNS 不存在")
	}
	if !PeerRouteHasAllMembers(rt.MemberUserIDs) {
		return fmt.Errorf("仅「全部账号」适用范围可配置排除名单")
	}
	excludes := NormalizeExcludeUserIDs(excludeIDs)
	if err := s.validateDNSExcludeIDs(excludes); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dns_server_excludes WHERE dns_id=?`, id); err != nil {
		return err
	}
	if err := insertDNSExcludesTx(tx, id, excludes); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE dns_servers SET updated_at=datetime('now') WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteDNSServer 删除托管 DNS（config 行须由上层拒绝；CASCADE 清成员/排除）。
func (s *Store) DeleteDNSServer(id int64) (*DNSServer, error) {
	old, err := s.GetDNSServer(id)
	if err != nil {
		return nil, err
	}
	if old == nil {
		return nil, fmt.Errorf("托管 DNS 不存在")
	}
	if old.IsConfigSource() {
		return nil, fmt.Errorf("配置文件 DNS 不可删除（请从 server.yaml 移除）")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dns_server_members WHERE dns_id=?`, id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM dns_server_excludes WHERE dns_id=?`, id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM dns_servers WHERE id=?`, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return old, nil
}

// GetDNSServer 按 id 取完整行（含 members/excludes）；不存在返回 nil,nil。
func (s *Store) GetDNSServer(id int64) (*DNSServer, error) {
	row := s.db.QueryRow(
		`SELECT id, dns_ip, remark, source, created_at, updated_at FROM dns_servers WHERE id=?`, id)
	d, err := scanDNSServer(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadDNSBindings(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// ListDNSServers 列出全部托管 DNS（新→旧），含绑定。
func (s *Store) ListDNSServers() ([]DNSServer, error) {
	rows, err := s.db.Query(
		`SELECT id, dns_ip, remark, source, created_at, updated_at FROM dns_servers ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DNSServer
	for rows.Next() {
		d, err := scanDNSServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadDNSBindings(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ListDNSServersPage 分页列出托管 DNS；total 为全表行数。
func (s *Store) ListDNSServersPage(limit, offset int) ([]DNSServer, int, error) {
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM dns_servers`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(
		`SELECT id, dns_ip, remark, source, created_at, updated_at FROM dns_servers ORDER BY id DESC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []DNSServer
	for rows.Next() {
		d, err := scanDNSServer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		if err := s.loadDNSBindings(&out[i]); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// ListDNSIPsForUser 返回应对该账号下发的 DNS IP 列表（members−excludes，按 id 升序稳定）。
func (s *Store) ListDNSIPsForUser(userID int64) ([]string, error) {
	if userID <= 0 {
		return nil, nil
	}
	all, err := s.ListDNSServers()
	if err != nil {
		return nil, err
	}
	// 按 id 升序保证握手稳定
	type pair struct {
		id int64
		ip string
	}
	var hit []pair
	for _, d := range all {
		if DNSAppliesToUser(d.MemberUserIDs, d.ExcludeUserIDs, userID) {
			hit = append(hit, pair{id: d.ID, ip: d.DNSIP})
		}
	}
	for i := 0; i < len(hit); i++ {
		for j := i + 1; j < len(hit); j++ {
			if hit[j].id < hit[i].id {
				hit[i], hit[j] = hit[j], hit[i]
			}
		}
	}
	out := make([]string, 0, len(hit))
	seen := map[string]struct{}{}
	for _, p := range hit {
		if _, ok := seen[p.ip]; ok {
			continue
		}
		seen[p.ip] = struct{}{}
		out = append(out, p.ip)
	}
	return out, nil
}

// insertDNSMembersTx 在事务内写入包含集（调用方已规范化）。
func insertDNSMembersTx(tx *sql.Tx, dnsID int64, members []int64) error {
	for _, mid := range members {
		if _, err := tx.Exec(`INSERT INTO dns_server_members(dns_id, user_id) VALUES(?,?)`, dnsID, mid); err != nil {
			return fmt.Errorf("插入 dns_server_members: %w", err)
		}
	}
	return nil
}

// insertDNSExcludesTx 在事务内写入排除集（调用方已规范化；可空）。
func insertDNSExcludesTx(tx *sql.Tx, dnsID int64, excludes []int64) error {
	for _, eid := range excludes {
		if _, err := tx.Exec(`INSERT INTO dns_server_excludes(dns_id, user_id) VALUES(?,?)`, dnsID, eid); err != nil {
			return fmt.Errorf("插入 dns_server_excludes: %w", err)
		}
	}
	return nil
}

// loadDNSBindings 填充 DNSServer 的 MemberUserIDs / ExcludeUserIDs。
func (s *Store) loadDNSBindings(d *DNSServer) error {
	members, err := s.listDNSMembers(d.ID)
	if err != nil {
		return err
	}
	excludes, err := s.listDNSExcludes(d.ID)
	if err != nil {
		return err
	}
	d.MemberUserIDs = members
	d.ExcludeUserIDs = excludes
	return nil
}

// listDNSMembers 读取某 DNS 的包含集 user_id（含 0=全部哨兵）。
func (s *Store) listDNSMembers(dnsID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT user_id FROM dns_server_members WHERE dns_id=? ORDER BY user_id`, dnsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// listDNSExcludes 读取某 DNS 的排除 user_id。
func (s *Store) listDNSExcludes(dnsID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT user_id FROM dns_server_excludes WHERE dns_id=? ORDER BY user_id`, dnsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// dnsScanner 抽象 *sql.Row / Rows，便于 scanDNSServer 复用。
type dnsScanner interface {
	Scan(dest ...any) error
}

// scanDNSServer 扫描 dns_servers 主表列（不含绑定）。
func scanDNSServer(row dnsScanner) (DNSServer, error) {
	var d DNSServer
	var created, updated string
	if err := row.Scan(&d.ID, &d.DNSIP, &d.Remark, &d.Source, &created, &updated); err != nil {
		return d, err
	}
	d.CreatedAt = timeutil.ParseUTC(created)
	d.UpdatedAt = timeutil.ParseUTC(updated)
	return d, nil
}
