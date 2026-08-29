package persist

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	"haovpn/internal/timeutil"
)

// PeerAccess 账号互访白名单一行：访问方可发往对方当前 VPN IP。
type PeerAccess struct {
	UserID     int64     `json:"user_id"`
	PeerUserID int64     `json:"peer_user_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// PeerRouteMemberAll 访问方 user_id=0 表示「全部账号」。
const PeerRouteMemberAll int64 = 0

// PeerRoute 托管路由定义一行（dest via via_user）；访问方在 MemberUserIDs。
//
// MemberUserIDs 含 0 表示全部；解析策略时若存在 0 则忽略同路由下其它指定。
type PeerRoute struct {
	ID            int64     `json:"id"`
	DestCIDR      string    `json:"dest_cidr"`
	ViaUserID     int64     `json:"via_user_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	MemberUserIDs []int64   `json:"member_user_ids,omitempty"` // 0=全部
}

// ValidatePeerRouteDest 校验托管路由目标 CIDR：须合法且禁止默认路由。
func ValidatePeerRouteDest(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return fmt.Errorf("dest_cidr 不能为空")
	}
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		ip := net.ParseIP(cidr)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("无效 dest_cidr: %s", cidr)
		}
		return nil
	}
	ones, bits := n.Mask.Size()
	if bits == 32 && ones == 0 {
		return fmt.Errorf("禁止默认路由 0.0.0.0/0（托管路由）")
	}
	return nil
}

// NormalizePeerRouteDest 将单 IP 规范为 /32，已是 CIDR 则原样返回规范化字符串。
func NormalizePeerRouteDest(cidr string) (string, error) {
	cidr = strings.TrimSpace(cidr)
	if err := ValidatePeerRouteDest(cidr); err != nil {
		return "", err
	}
	if _, n, err := net.ParseCIDR(cidr); err == nil {
		return n.String(), nil
	}
	ip := net.ParseIP(cidr).To4()
	return ip.String() + "/32", nil
}

// NormalizeMemberUserIDs 规范化访问方列表：去重；若含全部(0)则只保留 0。
func NormalizeMemberUserIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	var out []int64
	hasAll := false
	for _, id := range ids {
		if id < 0 {
			continue
		}
		if id == PeerRouteMemberAll {
			hasAll = true
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if hasAll {
		return []int64{PeerRouteMemberAll}
	}
	return out
}

// PeerRouteHasAllMembers 是否包含「全部」访问方。
func PeerRouteHasAllMembers(ids []int64) bool {
	for _, id := range ids {
		if id == PeerRouteMemberAll {
			return true
		}
	}
	return false
}

// AddPeerAccess 添加互访白名单；user 与 peer 不得相同。
func (s *Store) AddPeerAccess(userID, peerUserID int64) error {
	if userID <= 0 || peerUserID <= 0 || userID == peerUserID {
		return fmt.Errorf("无效的互访账号对")
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO peer_access(user_id, peer_user_id) VALUES(?,?)`,
		userID, peerUserID,
	)
	return err
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

// InsertPeerRoute 新增托管路由定义并写入访问方；memberIDs 空则报错。
//
// memberIDs 含 0 表示全部；与指定并存时 Normalize 后只保留 0。
func (s *Store) InsertPeerRoute(destCIDR string, viaUserID int64, memberIDs []int64) (int64, error) {
	dest, err := NormalizePeerRouteDest(destCIDR)
	if err != nil {
		return 0, err
	}
	if viaUserID <= 0 {
		return 0, fmt.Errorf("via_user_id 无效")
	}
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return 0, fmt.Errorf("须指定至少一个访问方（或全部）")
	}
	for _, mid := range members {
		if mid == PeerRouteMemberAll {
			continue
		}
		if mid == viaUserID {
			return 0, fmt.Errorf("via 不能是访问方自己")
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO peer_routes(dest_cidr, via_user_id) VALUES(?,?)`,
		dest, viaUserID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, fmt.Errorf("相同目标网段与 via 已存在")
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for _, mid := range members {
		if _, err := tx.Exec(`INSERT INTO peer_route_members(route_id, user_id) VALUES(?,?)`, id, mid); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// ReplacePeerRouteMembers 替换某路由的访问方列表。
func (s *Store) ReplacePeerRouteMembers(routeID int64, memberIDs []int64) error {
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return fmt.Errorf("须指定至少一个访问方（或全部）")
	}
	rt, err := s.GetPeerRoute(routeID)
	if err != nil || rt == nil {
		return fmt.Errorf("路由不存在")
	}
	for _, mid := range members {
		if mid != PeerRouteMemberAll && mid == rt.ViaUserID {
			return fmt.Errorf("via 不能是访问方自己")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM peer_route_members WHERE route_id=?`, routeID); err != nil {
		return err
	}
	for _, mid := range members {
		if _, err := tx.Exec(`INSERT INTO peer_route_members(route_id, user_id) VALUES(?,?)`, routeID, mid); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE peer_routes SET updated_at=datetime('now') WHERE id=?`, routeID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePeerRoute 按主键删除托管路由及其成员。
func (s *Store) DeletePeerRoute(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM peer_route_members WHERE route_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM peer_routes WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// GetPeerRoute 按 id 取一行（含成员）；不存在返回 (nil, nil)。
func (s *Store) GetPeerRoute(id int64) (*PeerRoute, error) {
	row := s.db.QueryRow(
		`SELECT id, dest_cidr, via_user_id, created_at, updated_at FROM peer_routes WHERE id=?`, id)
	r, err := scanPeerRoute(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	members, err := s.ListPeerRouteMembers(id)
	if err != nil {
		return nil, err
	}
	r.MemberUserIDs = members
	return r, nil
}

// ListPeerRouteMembers 列出路由访问方 user_id（含 0）。
func (s *Store) ListPeerRouteMembers(routeID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT user_id FROM peer_route_members WHERE route_id=? ORDER BY user_id`, routeID)
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

// ListPeerRoutes 列出全部托管路由（含成员，控制台用）。
func (s *Store) ListPeerRoutes() ([]PeerRoute, error) {
	rows, err := s.db.Query(
		`SELECT id, dest_cidr, via_user_id, created_at, updated_at FROM peer_routes ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanPeerRouteRows(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		members, err := s.ListPeerRouteMembers(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].MemberUserIDs = members
	}
	return out, nil
}

// ListPeerRoutesForAccessor 握手用：全部绑定 或 指定本用户的路由。
//
// 同一路由若同时有 0 与指定，仍命中（全部优先语义在策略层展示）。
func (s *Store) ListPeerRoutesForAccessor(userID int64) ([]PeerRoute, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT r.id, r.dest_cidr, r.via_user_id, r.created_at, r.updated_at
		 FROM peer_routes r
		 INNER JOIN peer_route_members m ON m.route_id = r.id
		 WHERE m.user_id = ? OR m.user_id = ?
		 ORDER BY r.id`, PeerRouteMemberAll, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanPeerRouteRows(rows)
	if err != nil {
		return nil, err
	}
	for i := range out {
		members, err := s.ListPeerRouteMembers(out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].MemberUserIDs = members
	}
	return out, nil
}

// HasLanRegistryMatch via 账号是否在注册表登记了该 dest（有效出口广告）。
func (s *Store) HasLanRegistryMatch(viaUserID int64, destCIDR string) (bool, error) {
	dest, err := NormalizePeerRouteDest(destCIDR)
	if err != nil {
		return false, err
	}
	var n int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM client_lan_registry WHERE user_id=? AND dest_cidr=?`,
		viaUserID, dest,
	).Scan(&n)
	return n > 0, err
}

type peerRouteScanner interface {
	Scan(dest ...any) error
}

func scanPeerRoute(sc peerRouteScanner) (*PeerRoute, error) {
	var r PeerRoute
	var created, updated string
	if err := sc.Scan(&r.ID, &r.DestCIDR, &r.ViaUserID, &created, &updated); err != nil {
		return nil, err
	}
	r.CreatedAt = timeutil.ParseUTC(created)
	r.UpdatedAt = timeutil.ParseUTC(updated)
	return &r, nil
}

func scanPeerRouteRows(rows *sql.Rows) ([]PeerRoute, error) {
	var out []PeerRoute
	for rows.Next() {
		r, err := scanPeerRoute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}
