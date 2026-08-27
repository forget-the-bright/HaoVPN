package persist

import (
	"time"
)

// RecordIPAllocation 记录 IP 池占用。
//
// 参数：ip — 分配的 VPN IPv4；userID — 占用者 users.id。
// 返回：err 为 INSERT/UPSERT 失败（唯一约束冲突时更新占用者）。
// 副作用：写 ip_allocations 表；allocated_at 置当前时间，released_at/lease_until 清空。
// 并发：由 vpnaccount 在 IP 分配时调用；与其它写操作 SQLite 串行化。
func (s *Store) RecordIPAllocation(ip string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO ip_allocations(ip, user_id, allocated_at, released_at, lease_until) VALUES(?,?,datetime('now'),NULL,NULL)
		ON CONFLICT(ip) DO UPDATE SET user_id=excluded.user_id, allocated_at=datetime('now'), released_at=NULL, lease_until=NULL`, ip, userID)
	return err
}

// ReleaseIPAllocation 标记 IP 已回收。
//
// 参数：ip — 待释放的 VPN IPv4。
// 返回：err 为 UPDATE 失败；无匹配行时仍返回 nil（影响行数 0）。
// 副作用：写 ip_allocations.released_at=now、lease_until=NULL；仅更新尚未 released 的行。
// 并发：断线或租约到期清理时调用。
func (s *Store) ReleaseIPAllocation(ip string) error {
	_, err := s.db.Exec(`UPDATE ip_allocations SET released_at=datetime('now'), lease_until=NULL WHERE ip=? AND released_at IS NULL`, ip)
	return err
}

// SetIPLeaseUntil 动态租约模式：断线后保留 IP 至指定时间。
//
// 参数：ip — 租约中的 VPN IP；until — 租约到期 UTC 时间。
// 返回：err 为 UPDATE 失败。
// 副作用：写 ip_allocations.lease_until；dynamic_lease 断线后由 sessionmgr 调用。
// 并发：与其它 IP 写操作串行。
func (s *Store) SetIPLeaseUntil(ip string, until time.Time) error {
	_, err := s.db.Exec(`UPDATE ip_allocations SET lease_until=? WHERE ip=?`, until.UTC().Format("2006-01-02 15:04:05"), ip)
	return err
}

// GetLeasedIPForUser 返回该用户仍占用的 IP（在线或租约未过期）。
//
// 参数：userID — users.id。
// 返回：ip 为最近分配且未 released、且 lease_until 为空或晚于 now 的地址；
// err 为 sql.ErrNoRows（无占用）或查询失败。
// 副作用：只读 ip_allocations 表。
// 并发：握手 IP 复用时调用；只读无锁。
func (s *Store) GetLeasedIPForUser(userID int64) (string, error) {
	var ip string
	err := s.db.QueryRow(`SELECT ip FROM ip_allocations WHERE user_id=? AND released_at IS NULL
		AND (lease_until IS NULL OR lease_until > datetime('now'))
		ORDER BY allocated_at DESC LIMIT 1`, userID).Scan(&ip)
	if err != nil {
		return "", err
	}
	return ip, nil
}

// ListActiveUserIPs 返回当前仍占用的 VPN IP。
//
// 返回：map[ip]userID，仅含 released_at IS NULL 的行；err 为查询或 Scan 失败。
// 副作用：只读 ip_allocations 表；ippool 启动时重建内存池使用。
// 并发：可并行调用；只读无锁。
func (s *Store) ListActiveUserIPs() (map[string]int64, error) {
	rows, err := s.db.Query(`SELECT ip, user_id FROM ip_allocations WHERE released_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var ip string
		var userID int64
		if err := rows.Scan(&ip, &userID); err != nil {
			return nil, err
		}
		out[ip] = userID
	}
	return out, rows.Err()
}

// ExpireLeasedIPs 清理已过期的租约 IP（released_at 置位）。
//
// 返回：n 为本次标记释放的行数；err 为 UPDATE 失败。
// 副作用：写 ip_allocations.released_at；lease_until≤now 且尚未 released 的行。
// 并发：定时任务调用；须在 ListExpiredLeasedIPs 之后或与之间协调。
func (s *Store) ExpireLeasedIPs() (int, error) {
	res, err := s.db.Exec(`UPDATE ip_allocations SET released_at=datetime('now')
		WHERE lease_until IS NOT NULL AND lease_until <= datetime('now') AND released_at IS NULL`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListExpiredLeasedIPs 返回当前已过期且尚未 released 的租约 IP（须在 ExpireLeasedIPs 前调用）。
//
// 返回：[]ip 待清理地址列表；err 为查询失败。
// 副作用：只读 ip_allocations 表；供 sessionmgr 在释放前踢掉对应会话。
// 并发：定时任务单 goroutine 调用。
func (s *Store) ListExpiredLeasedIPs() ([]string, error) {
	rows, err := s.db.Query(`SELECT ip FROM ip_allocations
		WHERE lease_until IS NOT NULL AND lease_until <= datetime('now') AND released_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// GetUserVPNIPIndex 返回所有账号 vpn_ip→user_id（横向隔离索引）。
//
// 返回：map[vpnIP]userID，仅含 vpn_ip 非空的 VPN 账号；err 为 ListVPNAccounts 失败。
// 副作用：只读 users 表（经 ListVPNAccounts）。
// 并发：sessionmgr 包过滤跨用户流量时调用；可并行只读。
func (s *Store) GetUserVPNIPIndex() (map[string]int64, error) {
	users, err := s.ListVPNAccounts()
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(users))
	for _, u := range users {
		if u.VPNIP != "" {
			out[u.VPNIP] = u.ID
		}
	}
	return out, nil
}

// ResolveAllowedIPs 解析账号有效 AllowedIPs（空数组用默认模板）。
//
// 参数：stored — 库中 JSON 反序列化结果；defaults — 服务端默认分流 CIDR。
// 返回：stored 非空时原样返回副本；否则返回 defaults 的浅拷贝。
// 副作用：无；纯函数。
// 并发：任意 goroutine 可并行调用。
func ResolveAllowedIPs(stored []string, defaults []string) []string {
	if len(stored) == 0 {
		out := make([]string, len(defaults))
		copy(out, defaults)
		return out
	}
	return stored
}
