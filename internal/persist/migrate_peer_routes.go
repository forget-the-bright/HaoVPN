package persist

import (
	"database/sql"
	"fmt"
	"strings"

	"haovpn/internal/logger"
)

// schemaMetaPeerRoutesV2 标记 peer_routes 已迁到「定义 + members」模型。
const schemaMetaPeerRoutesV2 = "peer_routes_v2"

// migratePeerRoutesV2 将旧版 peer_routes(user_id 访问方) 迁到定义表 + peer_route_members。
//
// 旧库：peer_routes 含 user_id（NULL=全员）；新库 schema.sql 已无该列。
// 幂等：schema_meta peer_routes_v2=1 或已无 user_id 列时直接跳过。
func (s *Store) migratePeerRoutesV2() error {
	v, err := s.GetSetting(schemaMetaPeerRoutesV2)
	if err != nil {
		return err
	}
	if v == "1" {
		return nil
	}

	hasUserCol, err := s.tableHasColumn("peer_routes", "user_id")
	if err != nil {
		return err
	}
	if !hasUserCol {
		// 新库或已手工对齐：仅打标
		return s.SetSetting(schemaMetaPeerRoutesV2, "1")
	}

	logger.Info("persist: 开始迁移 peer_routes → 定义+members")

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 迁移期间关闭外键，避免 DROP 旧 peer_routes 时 CASCADE 清掉已写入的 members
	if _, err := tx.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE IF EXISTS peer_route_members`); err != nil {
		return fmt.Errorf("drop peer_route_members: %w", err)
	}
	if _, err := tx.Exec(`
CREATE TABLE peer_route_members (
    route_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    PRIMARY KEY (route_id, user_id)
)`); err != nil {
		return fmt.Errorf("create peer_route_members: %w", err)
	}

	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS peer_routes_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dest_cidr TEXT NOT NULL,
    via_user_id INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (dest_cidr, via_user_id)
)`); err != nil {
		return fmt.Errorf("create peer_routes_v2: %w", err)
	}

	rows, err := tx.Query(`SELECT id, user_id, dest_cidr, via_user_id, created_at, updated_at FROM peer_routes ORDER BY id`)
	if err != nil {
		return fmt.Errorf("scan old peer_routes: %w", err)
	}

	// key = dest|via → new route id；合并同一 dest+via 的多访问方
	type routeKey struct {
		dest string
		via  int64
	}
	newIDByKey := map[routeKey]int64{}

	for rows.Next() {
		var oldID, viaID int64
		var uid sql.NullInt64
		var dest, created, updated string
		if err := rows.Scan(&oldID, &uid, &dest, &viaID, &created, &updated); err != nil {
			rows.Close()
			return err
		}
		dest = strings.TrimSpace(dest)
		k := routeKey{dest: dest, via: viaID}
		newID, ok := newIDByKey[k]
		if !ok {
			res, err := tx.Exec(
				`INSERT INTO peer_routes_v2(dest_cidr, via_user_id, created_at, updated_at) VALUES(?,?,?,?)`,
				dest, viaID, created, updated,
			)
			if err != nil {
				rows.Close()
				return fmt.Errorf("insert peer_routes_v2: %w", err)
			}
			newID, err = res.LastInsertId()
			if err != nil {
				rows.Close()
				return err
			}
			newIDByKey[k] = newID
		}
		memberUID := int64(0) // NULL → 全部
		if uid.Valid {
			memberUID = uid.Int64
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO peer_route_members(route_id, user_id) VALUES(?,?)`,
			newID, memberUID,
		); err != nil {
			rows.Close()
			return fmt.Errorf("insert member: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if _, err := tx.Exec(`DROP TABLE peer_routes`); err != nil {
		return fmt.Errorf("drop old peer_routes: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE peer_routes_v2 RENAME TO peer_routes`); err != nil {
		return fmt.Errorf("rename peer_routes_v2: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_peer_routes_via ON peer_routes(via_user_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_peer_route_members_user ON peer_route_members(user_id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_meta(key, value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		schemaMetaPeerRoutesV2, "1",
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logger.Info("persist: peer_routes 迁移完成 routes=%d", len(newIDByKey))
	return nil
}

// tableHasColumn 用 PRAGMA table_info 判断列是否存在。
func (s *Store) tableHasColumn(table, col string) (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		// 表不存在
		if strings.Contains(err.Error(), "no such table") {
			return false, nil
		}
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, col) {
			return true, nil
		}
	}
	return false, rows.Err()
}
