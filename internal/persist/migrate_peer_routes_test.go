package persist_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
	_ "modernc.org/sqlite"
)

// TestMigratePeerRoutesV2FromLegacy 模拟旧库含 user_id 列，Open 后应迁到 members。
func TestMigratePeerRoutesV2FromLegacy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// 最小旧结构：users + 旧 peer_routes
	_, err = db.Exec(`
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL DEFAULT '',
  is_admin INTEGER NOT NULL DEFAULT 0,
  must_change_password INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  public_key TEXT NOT NULL DEFAULT '',
  private_key_enc TEXT NOT NULL DEFAULT '',
  vpn_ip TEXT,
  allowed_ips TEXT NOT NULL DEFAULT '[]',
  ip_mode TEXT NOT NULL DEFAULT 'fixed',
  ip_lease_sec INTEGER NOT NULL DEFAULT 86400,
  policy_ver INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE peer_routes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER REFERENCES users(id),
  dest_cidr TEXT NOT NULL,
  via_user_id INTEGER NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HashPassword("Pass12345!")
	res, err := db.Exec(`INSERT INTO users(username, password_hash, public_key, private_key_enc, vpn_ip, enabled)
		VALUES('via',?, 'pk','sk','10.88.0.2',1)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	viaID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO users(username, password_hash, public_key, private_key_enc, vpn_ip, enabled)
		VALUES('acc',?, 'pk2','sk2','10.88.0.3',1)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	accID, _ := res.LastInsertId()
	_, err = db.Exec(`INSERT INTO peer_routes(user_id, dest_cidr, via_user_id) VALUES(?,?,?)`, accID, "192.168.0.0/24", viaID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO peer_routes(user_id, dest_cidr, via_user_id) VALUES(NULL,?,?)`, "10.0.0.0/24", viaID)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	store, err := persist.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	routes, err := store.ListPeerRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("want 2 routes after migrate, got %d", len(routes))
	}
	got, err := store.ListPeerRoutesForAccessor(accID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("accessor should see all+own, got %d", len(got))
	}
	v, err := store.GetSetting("peer_routes_v2")
	if err != nil || v != "1" {
		t.Fatalf("meta peer_routes_v2=%q err=%v", v, err)
	}
}
