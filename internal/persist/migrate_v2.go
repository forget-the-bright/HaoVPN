package persist

import (
	"database/sql"
	"fmt"

	"haovpn/internal/logger"
)

const schemaVersionV2 = "2"

// ensureSchemaMeta 保证版本表存在（早期库可能没有该表）。
func (s *Store) ensureSchemaMeta() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	return err
}

// migrateV1ToV2 将 v1（users+peers 分表）迁移为 v2（VPN 字段并入 users）。
func (s *Store) migrateV1ToV2() error {
	if err := s.ensureSchemaMeta(); err != nil {
		return fmt.Errorf("ensure schema_meta: %w", err)
	}

	var ver string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&ver)
	if err == nil && ver == schemaVersionV2 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// 检测 v1 peers 表是否存在
	var name string
	err = s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='peers'`).Scan(&name)
	hasPeers := err == nil

	if hasPeers {
		logger.Info("检测到 v1 数据库，开始迁移至 v2（VPN 账号合一）…")
		if err := s.addVPNColumnsToUsers(); err != nil {
			return fmt.Errorf("add vpn columns: %w", err)
		}
		if err := s.mergePeersIntoUsers(); err != nil {
			return fmt.Errorf("merge peers: %w", err)
		}
		if err := s.migratePeerIDToUserID("connection_events"); err != nil {
			return fmt.Errorf("migrate connection_events: %w", err)
		}
		if err := s.migratePeerIDToUserID("session_stats"); err != nil {
			return fmt.Errorf("migrate session_stats: %w", err)
		}
		if err := s.migrateIPAllocations(); err != nil {
			return fmt.Errorf("migrate ip_allocations: %w", err)
		}
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS peers`); err != nil {
			return fmt.Errorf("drop peers: %w", err)
		}
		logger.Info("v1→v2 迁移完成，peers 表已删除")
	} else {
		// 新库或已是 v2 结构：确保 users 有 VPN 列
		if err := s.addVPNColumnsToUsers(); err != nil {
			return err
		}
	}

	_, err = s.db.Exec(`INSERT INTO schema_meta(key, value) VALUES('version', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, schemaVersionV2)
	return err
}

func (s *Store) addVPNColumnsToUsers() error {
	cols := []string{
		`ALTER TABLE users ADD COLUMN public_key TEXT`,
		`ALTER TABLE users ADD COLUMN private_key_enc TEXT`,
		`ALTER TABLE users ADD COLUMN vpn_ip TEXT`,
		`ALTER TABLE users ADD COLUMN allowed_ips TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE users ADD COLUMN ip_mode TEXT NOT NULL DEFAULT 'fixed'`,
		`ALTER TABLE users ADD COLUMN ip_lease_sec INTEGER NOT NULL DEFAULT 86400`,
		`ALTER TABLE users ADD COLUMN policy_ver INTEGER NOT NULL DEFAULT 1`,
	}
	for _, q := range cols {
		_, _ = s.db.Exec(q) // 列已存在则忽略
	}
	_, _ = s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_public_key ON users(public_key) WHERE public_key IS NOT NULL AND public_key != ''`)
	return nil
}

// mergePeersIntoUsers 每个 user_id 保留一条 peer（优先 enabled=1 且 id 最大）。
// 注意：须先读完再写，避免 MaxOpenConns=1 下 cursor 未关导致死锁。
func (s *Store) mergePeersIntoUsers() error {
	rows, err := s.db.Query(`SELECT id, user_id, public_key, private_key_enc, vpn_ip, allowed_ips, enabled, created_at, updated_at
		FROM peers ORDER BY user_id, enabled DESC, id DESC`)
	if err != nil {
		return err
	}
	type peerRow struct {
		peerID, userID                     int64
		pubKey, privEnc, vpnIP, allowedIPs string
		enabled                            int
	}
	var list []peerRow
	for rows.Next() {
		var r peerRow
		var created, updated string
		if err := rows.Scan(&r.peerID, &r.userID, &r.pubKey, &r.privEnc, &r.vpnIP, &r.allowedIPs, &r.enabled, &created, &updated); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	seen := map[int64]bool{}
	for _, r := range list {
		if seen[r.userID] {
			logger.Warn("迁移跳过重复 peer user_id=%d peer_id=%d", r.userID, r.peerID)
			continue
		}
		seen[r.userID] = true
		_, err := s.db.Exec(`UPDATE users SET public_key=?, private_key_enc=?, vpn_ip=?, allowed_ips=?,
			ip_mode='fixed', ip_lease_sec=86400, policy_ver=1, updated_at=datetime('now')
			WHERE id=?`, r.pubKey, r.privEnc, r.vpnIP, r.allowedIPs, r.userID)
		if err != nil {
			return fmt.Errorf("merge peer %d into user %d: %w", r.peerID, r.userID, err)
		}
	}
	return nil
}

func (s *Store) migratePeerIDToUserID(table string) error {
	if table == "connection_events" {
		if s.tableHasColumn("connection_events", "user_id") {
			return nil
		}
		if !s.tableHasColumn("connection_events", "peer_id") {
			return nil
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		_, err = tx.Exec(`CREATE TABLE connection_events_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id),
			event_type TEXT NOT NULL,
			remote_addr TEXT,
			detail_json TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO connection_events_new(id, user_id, event_type, remote_addr, detail_json, created_at)
			SELECT ce.id, p.user_id, ce.event_type, ce.remote_addr, ce.detail_json, ce.created_at
			FROM connection_events ce JOIN peers p ON ce.peer_id = p.id`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DROP TABLE connection_events`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`ALTER TABLE connection_events_new RENAME TO connection_events`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`CREATE INDEX IF NOT EXISTS idx_connection_events_user_time ON connection_events(user_id, created_at)`)
		return tx.Commit()
	}

	if table == "session_stats" {
		if s.tableHasColumn("session_stats", "user_id") {
			return nil
		}
		if !s.tableHasColumn("session_stats", "peer_id") {
			return nil
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		_, err = tx.Exec(`CREATE TABLE session_stats_new (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			connected_at TEXT,
			last_heartbeat TEXT,
			rx_bytes INTEGER NOT NULL DEFAULT 0,
			tx_bytes INTEGER NOT NULL DEFAULT 0,
			reconnect_count INTEGER NOT NULL DEFAULT 0,
			remote_addr TEXT
		)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO session_stats_new(user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr)
			SELECT p.user_id, ss.connected_at, ss.last_heartbeat, ss.rx_bytes, ss.tx_bytes, ss.reconnect_count, ss.remote_addr
			FROM session_stats ss JOIN peers p ON ss.peer_id = p.id`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DROP TABLE session_stats`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`ALTER TABLE session_stats_new RENAME TO session_stats`)
		return tx.Commit()
	}
	return nil
}

func (s *Store) tableHasColumn(table, col string) bool {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == col {
			return true
		}
	}
	return false
}

func (s *Store) migrateIPAllocations() error {
	if s.tableHasColumn("ip_allocations", "user_id") {
		if !s.tableHasColumn("ip_allocations", "lease_until") {
			_, _ = s.db.Exec(`ALTER TABLE ip_allocations ADD COLUMN lease_until TEXT`)
		}
		return nil
	}
	if !s.tableHasColumn("ip_allocations", "peer_id") {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`CREATE TABLE ip_allocations_new (
		ip TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id),
		allocated_at TEXT NOT NULL DEFAULT (datetime('now')),
		released_at TEXT,
		lease_until TEXT
	)`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO ip_allocations_new(ip, user_id, allocated_at, released_at, lease_until)
		SELECT ia.ip, p.user_id, ia.allocated_at, ia.released_at, NULL
		FROM ip_allocations ia JOIN peers p ON ia.peer_id = p.id`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DROP TABLE ip_allocations`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`ALTER TABLE ip_allocations_new RENAME TO ip_allocations`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
