package persist

import (
	"fmt"
)

// CountUsers 统计 users 表总行数（含禁用与纯 Web 账号）。
//
// 返回：n 为账号总数；err 为查询失败（库已关闭、磁盘错误等）。
// 副作用：只读；常用于初始化时判断是否需创建默认管理员。
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CountEnabledAdmins 统计启用中的 Web 管理员数量（is_admin=1 AND enabled=1）。
//
// 用途：删除/禁用账号前防止锁死管理面（须至少保留一名启用管理员）。
// 关联：vpnaccount.DeleteAccount / SetAccountEnabled。
func (s *Store) CountEnabledAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1 AND enabled=1`).Scan(&n)
	return n, err
}

// CreateUser 仅创建 Web 账号（无隧道身份，如 admin）。
func (s *Store) CreateUser(username, passwordHash string, mustChange bool) (int64, error) {
	return s.CreateAdminUser(username, passwordHash, mustChange)
}

// CreateAdminUser 创建 Web 管理员账号（is_admin=1）。
func (s *Store) CreateAdminUser(username, passwordHash string, mustChange bool) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, must_change_password, is_admin) VALUES(?,?,?,1)`,
		username, passwordHash, boolToInt(mustChange))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateVPNAccount 创建带隧道身份的 VPN 账号（Web + 密钥 + IP 策略）。
func (s *Store) CreateVPNAccount(u User) (int64, error) {
	ips := marshalStringSlice(u.AllowedIPs)
	if u.IPMode == "" {
		u.IPMode = IPModeFixed
	}
	if u.IPLeaseSec <= 0 {
		u.IPLeaseSec = DefaultIPLeaseSec
	}
	if u.PolicyVer <= 0 {
		u.PolicyVer = 1
	}
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, must_change_password, public_key, private_key_enc,
		vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		u.Username, u.PasswordHash, boolToInt(u.MustChangePassword), nullStr(u.PublicKey), nullStr(u.PrivateKeyEnc),
		nullStr(u.VPNIP), ips, u.IPMode, u.IPLeaseSec, u.PolicyVer)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateVPNFields 更新账号隧道字段（策略/IP 模式等）。
func (s *Store) UpdateVPNFields(id int64, vpnIP string, allowedIPs []string, ipMode string, ipLeaseSec, policyVer int) error {
	ips := marshalStringSlice(allowedIPs)
	_, err := s.db.Exec(`UPDATE users SET vpn_ip=?, allowed_ips=?, ip_mode=?, ip_lease_sec=?, policy_ver=?, updated_at=datetime('now') WHERE id=?`,
		nullStr(vpnIP), ips, ipMode, ipLeaseSec, policyVer, id)
	return err
}

// UpdateUserVPNIP 仅更新 VPN IP（动态分配时）。
func (s *Store) UpdateUserVPNIP(id int64, vpnIP string) error {
	_, err := s.db.Exec(`UPDATE users SET vpn_ip=?, updated_at=datetime('now') WHERE id=?`, vpnIP, id)
	return err
}

// IncrementPolicyVer 策略变更时递增版本并返回新版本号。
func (s *Store) IncrementPolicyVer(id int64) (int, error) {
	_, err := s.db.Exec(`UPDATE users SET policy_ver=policy_ver+1, updated_at=datetime('now') WHERE id=?`, id)
	if err != nil {
		return 0, err
	}
	u, err := s.GetUserByID(id)
	if err != nil {
		return 0, err
	}
	return u.PolicyVer, nil
}

// GetUserByUsername 按登录名查询用户（Web 登录与隧道身份共用）。
//
// 参数：username — 非空、与 users.username 精确匹配。
// 返回：*User 含密码哈希等敏感字段；未找到时 err 为 sql.ErrNoRows。
// 副作用：只读。
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE username=?`, username)
	return scanUser(row)
}

// GetUserByID 按主键查询用户。
//
// 参数：id — users.id；须为正整数。
// 返回：*User；未找到时 err 为 sql.ErrNoRows。
// 副作用：只读；IncrementPolicyVer 等内部方法也会调用。
func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE id=?`, id)
	return scanUser(row)
}

// GetUserByPublicKey 隧道握手：按公钥查找 VPN 账号。
func (s *Store) GetUserByPublicKey(pub string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE public_key=?`, pub)
	return scanUser(row)
}

// ListUsers 列出全部用户，按 id 升序。
//
// 返回：[]User 含 Web 与 VPN 账号；无用户时为空切片非 nil；err 为查询失败。
// 副作用：只读；管理 API 用户列表与导出功能使用。
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userSelectCols + ` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// ListVPNAccounts 返回已配置隧道身份的账号。
func (s *Store) ListVPNAccounts() ([]User, error) {
	rows, err := s.db.Query(`SELECT ` + userSelectCols + ` FROM users WHERE public_key IS NOT NULL AND public_key != '' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateUserPassword 更新用户密码哈希并可选清除「须改密」标记。
//
// 参数：hash — 已哈希密码（如 bcrypt）；clearMustChange 为 true 时 must_change_password=0。
// 返回：err 为 UPDATE 失败或 id 不存在（影响行数 0 仍返回 nil）。
// 副作用：刷新 updated_at。
func (s *Store) UpdateUserPassword(id int64, hash string, clearMustChange bool) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=?, must_change_password=?, updated_at=datetime('now') WHERE id=?`,
		hash, boolToInt(!clearMustChange), id)
	return err
}

// SetUserEnabled 启用或禁用账号（Web 登录与隧道握手均受 enabled 约束）。
//
// 参数：enabled — false 时拒绝认证，不断开已有 TCP（由 sessionmgr 处理）。
// 返回：err 为 UPDATE 失败。
// 副作用：刷新 updated_at；禁用后新握手失败。
func (s *Store) SetUserEnabled(id int64, enabled bool) error {
	_, err := s.db.Exec(`UPDATE users SET enabled=?, updated_at=datetime('now') WHERE id=?`, boolToInt(enabled), id)
	return err
}

// DeleteUser 删除账号，并清理仍引用该 user_id 的子表（否则 SQLite FK 787）。
// audit_logs 仅把 actor_user_id 置空，保留审计痕迹。
func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// peer_* 语句含两个 ?，须各传一次 user id（modernc sqlite 不复用同一参数）
	type cascadeStmt struct {
		q    string
		args []any
	}
	stmts := []cascadeStmt{
		{`DELETE FROM peer_access WHERE user_id=? OR peer_user_id=?`, []any{id, id}},
		{`DELETE FROM peer_route_members WHERE user_id=? OR route_id IN (SELECT id FROM peer_routes WHERE via_user_id=?)`, []any{id, id}},
		{`DELETE FROM peer_routes WHERE via_user_id=?`, []any{id}},
		{`DELETE FROM client_lan_registry WHERE user_id=?`, []any{id}},
		{`DELETE FROM connection_events WHERE user_id=?`, []any{id}},
		{`DELETE FROM session_stats WHERE user_id=?`, []any{id}},
		{`DELETE FROM ip_allocations WHERE user_id=?`, []any{id}},
		{`UPDATE audit_logs SET actor_user_id=NULL WHERE actor_user_id=?`, []any{id}},
		{`DELETE FROM users WHERE id=?`, []any{id}},
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st.q, st.args...); err != nil {
			return fmt.Errorf("delete user cascade (%s): %w", st.q, err)
		}
	}
	return tx.Commit()
}

// UpdateUserPrivateKeyEnc 更新私钥密文。
func (s *Store) UpdateUserPrivateKeyEnc(id int64, enc string) error {
	_, err := s.db.Exec(`UPDATE users SET private_key_enc=?, updated_at=datetime('now') WHERE id=?`, enc, id)
	return err
}
