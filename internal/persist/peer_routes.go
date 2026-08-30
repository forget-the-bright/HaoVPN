package persist

import (
	"database/sql"
	"fmt"
	"strings"

	"haovpn/internal/timeutil"
)

// peer_routes.go：托管路由（peer_routes / peer_route_members）CRUD 与行扫描。

// InsertPeerRoute 新增托管路由定义并写入访问方；memberIDs 空则报错。
//
// memberIDs 含 0 表示全部；与指定并存时 Normalize 后只保留 0。
// via 与访问方（>0）须为已有 VPN 账号；错误文案稳定中文，避免裸 SQLite 串。
func (s *Store) InsertPeerRoute(destCIDR string, viaUserID int64, memberIDs []int64) (int64, error) {
	dest, err := NormalizePeerRouteDest(destCIDR)
	if err != nil {
		return 0, err
	}
	if viaUserID <= 0 {
		return 0, fmt.Errorf("via_user_id 无效")
	}
	via, err := s.GetUserByID(viaUserID)
	if err != nil {
		return 0, fmt.Errorf("查询 via 失败: %w", err)
	}
	if via == nil {
		return 0, fmt.Errorf("via 账号不存在")
	}
	if !via.HasVPN() {
		return 0, fmt.Errorf("via 须为 VPN 账号")
	}
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return 0, fmt.Errorf("须指定至少一个访问方（或全部）")
	}
	if err := s.validatePeerRouteMemberIDs(members, viaUserID); err != nil {
		return 0, err
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
//
// 调用方须在替换前读取旧成员并对 old∪new 打 dirty（本函数不负责踢线标记）。
func (s *Store) ReplacePeerRouteMembers(routeID int64, memberIDs []int64) error {
	members := NormalizeMemberUserIDs(memberIDs)
	if len(members) == 0 {
		return fmt.Errorf("须指定至少一个访问方（或全部）")
	}
	rt, err := s.GetPeerRoute(routeID)
	if err != nil || rt == nil {
		return fmt.Errorf("路由不存在")
	}
	if err := s.validatePeerRouteMemberIDs(members, rt.ViaUserID); err != nil {
		return err
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
