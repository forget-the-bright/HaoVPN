package persist

import (
	"database/sql"
	"errors"
	"fmt"

	"haovpn/internal/timeutil"
)

// peer_access.go：账号互访白名单（peer_access 表）的增删查。

// AddPeerAccess 添加互访白名单；双方须为已有 VPN 账号且不得相同。
//
// 校验存在性与 HasVPN，避免孤儿策略行与误导 UI；错误文案稳定中文。
func (s *Store) AddPeerAccess(userID, peerUserID int64) error {
	if userID <= 0 || peerUserID <= 0 || userID == peerUserID {
		return fmt.Errorf("无效的互访账号对")
	}
	if err := s.requireVPNUser(userID, "访问方"); err != nil {
		return err
	}
	if err := s.requireVPNUser(peerUserID, "对端"); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO peer_access(user_id, peer_user_id) VALUES(?,?)`,
		userID, peerUserID,
	)
	return err
}

// requireVPNUser 校验 userID 对应账号存在且为 VPN 账号（有公钥/vpn_ip 等）。
//
// 参数：role — 错误文案中的角色名（如「访问方」「对端」）。
func (s *Store) requireVPNUser(userID int64, role string) error {
	u, err := s.GetUserByID(userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s账号不存在", role)
		}
		return fmt.Errorf("查询%s失败", role)
	}
	if u == nil {
		return fmt.Errorf("%s账号不存在", role)
	}
	if !u.HasVPN() {
		return fmt.Errorf("%s须为 VPN 账号", role)
	}
	return nil
}

// RemovePeerAccess 删除一条互访白名单。
func (s *Store) RemovePeerAccess(userID, peerUserID int64) error {
	_, err := s.db.Exec(`DELETE FROM peer_access WHERE user_id=? AND peer_user_id=?`, userID, peerUserID)
	return err
}

// ListPeerAccessForUser 列出某访问方的互访白名单。
func (s *Store) ListPeerAccessForUser(userID int64) ([]PeerAccess, error) {
	rows, err := s.db.Query(
		`SELECT user_id, peer_user_id, created_at FROM peer_access WHERE user_id=? ORDER BY peer_user_id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeerAccess
	for rows.Next() {
		var a PeerAccess
		var created string
		if err := rows.Scan(&a.UserID, &a.PeerUserID, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = timeutil.ParseUTC(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// HasPeerAccess 访问方是否被允许直连对方（白名单）。
func (s *Store) HasPeerAccess(userID, peerUserID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM peer_access WHERE user_id=? AND peer_user_id=?`,
		userID, peerUserID,
	).Scan(&n)
	return n > 0, err
}

// ListPeerAccessPeerIDs 返回访问方可直连的 peer_user_id 列表。
func (s *Store) ListPeerAccessPeerIDs(userID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT peer_user_id FROM peer_access WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListAllPeerAccess 列出全部互访白名单（控制台一次查出，避免按用户 N+1）。
func (s *Store) ListAllPeerAccess() ([]PeerAccess, error) {
	rows, err := s.db.Query(
		`SELECT user_id, peer_user_id, created_at FROM peer_access ORDER BY user_id, peer_user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PeerAccess
	for rows.Next() {
		var a PeerAccess
		var created string
		if err := rows.Scan(&a.UserID, &a.PeerUserID, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = timeutil.ParseUTC(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// RemovePeerAccessPair 删除双向互访（A→B 与 B→A）。
func (s *Store) RemovePeerAccessPair(userID, peerUserID int64) error {
	_, err := s.db.Exec(
		`DELETE FROM peer_access WHERE (user_id=? AND peer_user_id=?) OR (user_id=? AND peer_user_id=?)`,
		userID, peerUserID, peerUserID, userID,
	)
	return err
}

// AddPeerAccessPair 写入双向互访（A→B 与 B→A）；已存在则忽略。
func (s *Store) AddPeerAccessPair(userID, peerUserID int64) error {
	if err := s.AddPeerAccess(userID, peerUserID); err != nil {
		return err
	}
	return s.AddPeerAccess(peerUserID, userID)
}
