// Package persist provides SQLite storage.
package persist

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"haovpn/internal/logger"
)

//go:embed schema.sql
var schemaFS embed.FS

// IP 分配模式常量
const (
	IPModeFixed          = "fixed"
	IPModeDynamicSession = "dynamic_session"
	IPModeDynamicLease   = "dynamic_lease"
)

// Store wraps SQLite operations.
type Store struct {
	db *sql.DB
}

// User VPN 账号：Web 登录 + 隧道身份合一。
type User struct {
	ID                 int64     `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	Enabled            bool      `json:"enabled"`
	MustChangePassword bool      `json:"must_change_password"`
	IsAdmin            bool      `json:"is_admin"`
	PublicKey          string    `json:"public_key,omitempty"`
	PrivateKeyEnc      string    `json:"-"`
	VPNIP              string    `json:"vpn_ip,omitempty"`
	AllowedIPs         []string  `json:"allowed_ips"`
	IPMode             string    `json:"ip_mode"`
	IPLeaseSec         int       `json:"ip_lease_sec"`
	PolicyVer          int       `json:"policy_ver"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// HasVPN 是否已配置隧道身份（有公钥）。
func (u *User) HasVPN() bool {
	return u != nil && u.PublicKey != ""
}

// AuditEntry is an audit log record.
type AuditEntry struct {
	ID          int64     `json:"id"`
	ActorUserID *int64    `json:"actor_user_id,omitempty"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    *int64    `json:"target_id,omitempty"`
	ClientIP    string    `json:"client_ip"`
	DetailJSON  string    `json:"detail_json,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ConnectionEvent 连接上下线事件。
type ConnectionEvent struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	EventType  string    `json:"event_type"`
	RemoteAddr string    `json:"remote_addr"`
	DetailJSON string    `json:"detail_json,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SessionStat 会话统计。
type SessionStat struct {
	UserID         int64
	ConnectedAt    *time.Time
	LastHeartbeat  *time.Time
	RxBytes        int64
	TxBytes        int64
	ReconnectCount int
	RemoteAddr     string
}

const userSelectCols = `id, username, password_hash, enabled, must_change_password, is_admin,
	public_key, private_key_enc, vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver, created_at, updated_at`

// Open opens or creates the SQLite database.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	logger.Info("sqlite opened: %s (WAL, busy_timeout=5000ms, max_open_conns=1)", path)
	return s, nil
}

func (s *Store) migrate() error {
	// v1 库若仍有 peers / peer_id 列，须先迁移再执行 schema（否则索引引用 user_id 会失败）
	if s.hasPeersTable() || s.tableHasColumn("connection_events", "peer_id") ||
		s.tableHasColumn("session_stats", "peer_id") || s.tableHasColumn("ip_allocations", "peer_id") {
		if err := s.migrateV1ToV2(); err != nil {
			return fmt.Errorf("migrate v1→v2: %w", err)
		}
	}
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := s.migrateV1ToV2(); err != nil {
		return err
	}
	return s.migrateV3()
}

func (s *Store) hasPeersTable() bool {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='peers'`).Scan(&name)
	return err == nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes raw db for transactions.
func (s *Store) DB() *sql.DB { return s.db }

// CountUsers returns user count.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
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
	ips, _ := json.Marshal(u.AllowedIPs)
	if u.IPMode == "" {
		u.IPMode = IPModeFixed
	}
	if u.IPLeaseSec <= 0 {
		u.IPLeaseSec = 86400
	}
	if u.PolicyVer <= 0 {
		u.PolicyVer = 1
	}
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, must_change_password, public_key, private_key_enc,
		vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		u.Username, u.PasswordHash, boolToInt(u.MustChangePassword), nullStr(u.PublicKey), nullStr(u.PrivateKeyEnc),
		nullStr(u.VPNIP), string(ips), u.IPMode, u.IPLeaseSec, u.PolicyVer)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateVPNFields 更新账号隧道字段（策略/IP 模式等）。
func (s *Store) UpdateVPNFields(id int64, vpnIP string, allowedIPs []string, ipMode string, ipLeaseSec, policyVer int) error {
	ips, _ := json.Marshal(allowedIPs)
	_, err := s.db.Exec(`UPDATE users SET vpn_ip=?, allowed_ips=?, ip_mode=?, ip_lease_sec=?, policy_ver=?, updated_at=datetime('now') WHERE id=?`,
		nullStr(vpnIP), string(ips), ipMode, ipLeaseSec, policyVer, id)
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

// GetUserByUsername finds a user by name.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE username=?`, username)
	return scanUser(row)
}

// GetUserByID finds a user by ID.
func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE id=?`, id)
	return scanUser(row)
}

// GetUserByPublicKey 隧道握手：按公钥查找 VPN 账号。
func (s *Store) GetUserByPublicKey(pub string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE public_key=?`, pub)
	return scanUser(row)
}

// ListUsers returns all users.
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

// UpdateUserPassword updates password hash.
func (s *Store) UpdateUserPassword(id int64, hash string, clearMustChange bool) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=?, must_change_password=?, updated_at=datetime('now') WHERE id=?`,
		hash, boolToInt(!clearMustChange), id)
	return err
}

// SetUserEnabled toggles user enabled state.
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

	stmts := []string{
		`DELETE FROM connection_events WHERE user_id=?`,
		`DELETE FROM session_stats WHERE user_id=?`,
		`DELETE FROM ip_allocations WHERE user_id=?`,
		`UPDATE audit_logs SET actor_user_id=NULL WHERE actor_user_id=?`,
		`DELETE FROM users WHERE id=?`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, id); err != nil {
			return fmt.Errorf("delete user cascade (%s): %w", q, err)
		}
	}
	return tx.Commit()
}

// UpdateUserPrivateKeyEnc 更新私钥密文。
func (s *Store) UpdateUserPrivateKeyEnc(id int64, enc string) error {
	_, err := s.db.Exec(`UPDATE users SET private_key_enc=?, updated_at=datetime('now') WHERE id=?`, enc, id)
	return err
}

// InsertAuditLog appends an audit record.
func (s *Store) InsertAuditLog(e AuditEntry) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(actor_user_id, action, target_type, target_id, client_ip, detail_json) VALUES(?,?,?,?,?,?)`,
		e.ActorUserID, e.Action, e.TargetType, e.TargetID, e.ClientIP, e.DetailJSON)
	return err
}

// ListAuditLogs returns recent audit entries.
func (s *Store) ListAuditLogs(limit, offset int) ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var actor sql.NullInt64
		var target sql.NullInt64
		var created string
		if err := rows.Scan(&e.ID, &actor, &e.Action, &e.TargetType, &target, &e.ClientIP, &e.DetailJSON, &created); err != nil {
			return nil, err
		}
		if actor.Valid {
			v := actor.Int64
			e.ActorUserID = &v
		}
		if target.Valid {
			v := target.Int64
			e.TargetID = &v
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// InsertConnectionEvent records a connection event.
func (s *Store) InsertConnectionEvent(userID int64, eventType, remoteAddr, detail string) error {
	_, err := s.db.Exec(`INSERT INTO connection_events(user_id, event_type, remote_addr, detail_json) VALUES(?,?,?,?)`,
		userID, eventType, remoteAddr, detail)
	return err
}

// ListRecentConnectionEvents 返回最近连接事件。
func (s *Store) ListRecentConnectionEvents(limit int) ([]ConnectionEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, user_id, event_type, remote_addr, detail_json, created_at
		FROM connection_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectionEvent
	for rows.Next() {
		var e ConnectionEvent
		var created string
		if err := rows.Scan(&e.ID, &e.UserID, &e.EventType, &e.RemoteAddr, &e.DetailJSON, &created); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertSessionStat updates session stats.
func (s *Store) UpsertSessionStat(st SessionStat) error {
	_, err := s.db.Exec(`INSERT INTO session_stats(user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET
		connected_at=excluded.connected_at,
		last_heartbeat=excluded.last_heartbeat,
		rx_bytes=excluded.rx_bytes,
		tx_bytes=excluded.tx_bytes,
		reconnect_count=excluded.reconnect_count,
		remote_addr=excluded.remote_addr`,
		st.UserID, timePtrStr(st.ConnectedAt), timePtrStr(st.LastHeartbeat),
		st.RxBytes, st.TxBytes, st.ReconnectCount, st.RemoteAddr)
	return err
}

// GetSessionStat returns stats for a user.
func (s *Store) GetSessionStat(userID int64) (*SessionStat, error) {
	row := s.db.QueryRow(`SELECT user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr FROM session_stats WHERE user_id=?`, userID)
	var st SessionStat
	var connAt, hb, remote sql.NullString
	if err := row.Scan(&st.UserID, &connAt, &hb, &st.RxBytes, &st.TxBytes, &st.ReconnectCount, &remote); err != nil {
		return nil, err
	}
	if connAt.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", connAt.String)
		st.ConnectedAt = &t
	}
	if hb.Valid {
		t, _ := time.Parse("2006-01-02 15:04:05", hb.String)
		st.LastHeartbeat = &t
	}
	if remote.Valid {
		st.RemoteAddr = remote.String
	}
	return &st, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (*User, error) {
	var u User
	var enabled, mustChange, isAdmin int
	var pubKey, privEnc, vpnIP, ipsJSON, ipMode sql.NullString
	var ipLease, policyVer int
	var created, updated string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &enabled, &mustChange, &isAdmin,
		&pubKey, &privEnc, &vpnIP, &ipsJSON, &ipMode, &ipLease, &policyVer, &created, &updated); err != nil {
		return nil, err
	}
	u.Enabled = enabled == 1
	u.MustChangePassword = mustChange == 1
	u.IsAdmin = isAdmin == 1
	u.PublicKey = pubKey.String
	u.PrivateKeyEnc = privEnc.String
	u.VPNIP = vpnIP.String
	if ipsJSON.Valid && ipsJSON.String != "" {
		_ = json.Unmarshal([]byte(ipsJSON.String), &u.AllowedIPs)
	}
	u.IPMode = ipMode.String
	if u.IPMode == "" {
		u.IPMode = IPModeFixed
	}
	u.IPLeaseSec = ipLease
	if u.IPLeaseSec <= 0 {
		u.IPLeaseSec = 86400
	}
	u.PolicyVer = policyVer
	if u.PolicyVer <= 0 {
		u.PolicyVer = 1
	}
	u.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", created)
	u.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updated)
	return &u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func timePtrStr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
