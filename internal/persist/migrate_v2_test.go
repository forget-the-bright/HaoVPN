package persist_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"haovpn/internal/persist"
)

// TestMigrateV1ToV2AccountMerge 旧 peers 表须合并进 users 并删除。
func TestMigrateV1ToV2AccountMerge(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  must_change_password INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE peers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  public_key TEXT NOT NULL UNIQUE,
  private_key_enc TEXT NOT NULL,
  vpn_ip TEXT NOT NULL,
  allowed_ips TEXT NOT NULL DEFAULT '[]',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE connection_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  peer_id INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  remote_addr TEXT,
  detail_json TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE session_stats (
  peer_id INTEGER PRIMARY KEY,
  connected_at TEXT,
  last_heartbeat TEXT,
  rx_bytes INTEGER NOT NULL DEFAULT 0,
  tx_bytes INTEGER NOT NULL DEFAULT 0,
  reconnect_count INTEGER NOT NULL DEFAULT 0,
  remote_addr TEXT
);
CREATE TABLE ip_allocations (
  ip TEXT PRIMARY KEY,
  peer_id INTEGER NOT NULL,
  allocated_at TEXT NOT NULL DEFAULT (datetime('now')),
  released_at TEXT
);
-- 刻意不建 schema_meta：与现场 home/data/haovpn.db 一致
INSERT INTO users(username, password_hash) VALUES('admin', 'hash-admin');
INSERT INTO users(username, password_hash) VALUES('eng1', 'hash-eng');
INSERT INTO peers(user_id, public_key, private_key_enc, vpn_ip, allowed_ips, enabled)
  VALUES(2, 'pub-eng', 'priv-eng', '10.88.0.10', '["192.168.1.0/24"]', 1);
INSERT INTO connection_events(peer_id, event_type, remote_addr) VALUES(1, 'connect', '1.2.3.4');
INSERT INTO session_stats(peer_id, rx_bytes, tx_bytes, reconnect_count) VALUES(1, 100, 200, 3);
INSERT INTO ip_allocations(ip, peer_id) VALUES('10.88.0.10', 1);
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var peersLeft int
	_ = store.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='peers'`).Scan(&peersLeft)
	if peersLeft != 0 {
		t.Fatal("peers 表应已删除")
	}

	u, err := store.GetUserByUsername("eng1")
	if err != nil {
		t.Fatal(err)
	}
	if u.PublicKey != "pub-eng" || u.VPNIP != "10.88.0.10" {
		t.Fatalf("peer 未合并: %+v", u)
	}
	if u.IPMode != persist.IPModeFixed || u.PolicyVer != 1 {
		t.Fatalf("默认策略字段: mode=%s ver=%d", u.IPMode, u.PolicyVer)
	}

	admin, err := store.GetUserByUsername("admin")
	if err != nil || admin.PublicKey != "" {
		t.Fatalf("admin 应保留且无 VPN: %+v %v", admin, err)
	}

	var ver string
	_ = store.DB().QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&ver)
	if ver != "3" {
		t.Fatalf("schema version=%s want 3", ver)
	}

	st, err := store.GetSessionStat(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.ReconnectCount != 3 || st.RxBytes != 100 {
		t.Fatalf("session_stats 迁移异常: %+v", st)
	}
}

// TestMigrateHomeLikeDBNoSchemaMeta 复现现场：有 peers、无 schema_meta 时 Open 不得 FATAL。
func TestMigrateHomeLikeDBNoSchemaMeta(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "home-like.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  must_change_password INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE peers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  public_key TEXT NOT NULL UNIQUE,
  private_key_enc TEXT NOT NULL,
  vpn_ip TEXT NOT NULL,
  allowed_ips TEXT NOT NULL DEFAULT '[]',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE connection_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  peer_id INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  remote_addr TEXT,
  detail_json TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE session_stats (
  peer_id INTEGER PRIMARY KEY,
  connected_at TEXT,
  last_heartbeat TEXT,
  rx_bytes INTEGER NOT NULL DEFAULT 0,
  tx_bytes INTEGER NOT NULL DEFAULT 0,
  reconnect_count INTEGER NOT NULL DEFAULT 0,
  remote_addr TEXT
);
CREATE TABLE ip_allocations (
  ip TEXT PRIMARY KEY,
  peer_id INTEGER NOT NULL,
  allocated_at TEXT NOT NULL DEFAULT (datetime('now')),
  released_at TEXT
);
CREATE TABLE audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor_user_id INTEGER,
  action TEXT NOT NULL,
  target_type TEXT,
  target_id INTEGER,
  client_ip TEXT,
  detail_json TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO users(username, password_hash) VALUES('admin', 'x');
INSERT INTO peers(user_id, public_key, private_key_enc, vpn_ip, allowed_ips)
  VALUES(1, 'pk', 'sk', '10.88.0.2', '[]');
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatalf("Open home-like db: %v", err)
	}
	defer store.Close()
	u, err := store.GetUserByID(1)
	if err != nil || u.VPNIP != "10.88.0.2" {
		t.Fatalf("merge failed: %+v %v", u, err)
	}
}
