package persist

import (
	"fmt"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/security"
)

// MigratePlaintextPrivateKeys 将历史明文 private_key_enc 批量 AES 加密。
func (s *Store) MigratePlaintextPrivateKeys(keyEnc *security.KeyEnc) error {
	if s == nil || keyEnc == nil {
		return nil
	}
	users, err := s.ListVPNAccounts()
	if err != nil {
		return err
	}
	var migrated int
	for _, u := range users {
		if u.PrivateKeyEnc == "" || security.IsEncryptedPrivateKey(u.PrivateKeyEnc) {
			continue
		}
		sealed, err := keyEnc.SealPrivateKey(u.PrivateKeyEnc)
		if err != nil {
			return fmt.Errorf("migrate user %d: %w", u.ID, err)
		}
		if err := s.UpdateUserPrivateKeyEnc(u.ID, sealed); err != nil {
			return err
		}
		migrated++
	}
	if migrated > 0 {
		logger.Info("已将 %d 个账号私钥从明文迁移为 AES 加密存储", migrated)
	}
	return nil
}

// RecordIPAllocation 记录 IP 池占用。
func (s *Store) RecordIPAllocation(ip string, userID int64) error {
	_, err := s.db.Exec(`INSERT INTO ip_allocations(ip, user_id, allocated_at, released_at, lease_until) VALUES(?,?,datetime('now'),NULL,NULL)
		ON CONFLICT(ip) DO UPDATE SET user_id=excluded.user_id, allocated_at=datetime('now'), released_at=NULL, lease_until=NULL`, ip, userID)
	return err
}

// ReleaseIPAllocation 标记 IP 已回收。
func (s *Store) ReleaseIPAllocation(ip string) error {
	_, err := s.db.Exec(`UPDATE ip_allocations SET released_at=datetime('now'), lease_until=NULL WHERE ip=? AND released_at IS NULL`, ip)
	return err
}

// SetIPLeaseUntil 动态租约模式：断线后保留 IP 至指定时间。
func (s *Store) SetIPLeaseUntil(ip string, until time.Time) error {
	_, err := s.db.Exec(`UPDATE ip_allocations SET lease_until=? WHERE ip=?`, until.UTC().Format("2006-01-02 15:04:05"), ip)
	return err
}

// GetLeasedIPForUser 返回该用户仍占用的 IP（在线或租约未过期）。
// 在线时 lease_until 可能为空；租约断线后 lease_until 有值。
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
func ResolveAllowedIPs(stored []string, defaults []string) []string {
	if len(stored) == 0 {
		out := make([]string, len(defaults))
		copy(out, defaults)
		return out
	}
	return stored
}
