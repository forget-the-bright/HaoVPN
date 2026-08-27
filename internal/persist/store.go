package persist

import (
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"haovpn/internal/logger"
)

//go:embed schema.sql
var schemaFS embed.FS

// IP 分配模式常量（users.ip_mode 取值）。
const (
	// IPModeFixed 创建账号时固定分配 vpn_ip，握手不复用池。
	IPModeFixed          = "fixed"
	// IPModeDynamicSession 断线立即释放 IP，下次握手重新分配。
	IPModeDynamicSession = "dynamic_session"
	// IPModeDynamicLease 断线后保留 IP 至 lease_until，租约内可复用。
	IPModeDynamicLease   = "dynamic_lease"
)

// Store 封装 SQLite 持久化操作，供服务端 Web/API、隧道握手与会话统计读写 users 等表。
//
// 字段：
//   db — 底层 *sql.DB 连接；Open 时设置 max_open_conns=1、WAL 与 busy_timeout。
//
// 线程安全：SQLite 单连接模式下由 Store 方法串行访问；事务须通过 DB() 自行管理。
type Store struct {
	db *sql.DB
}

// User VPN 账号：Web 登录与隧道身份合一，对应 users 表一行。
//
// 字段：
//   ID — 主键；CreateUser/CreateVPNAccount 写入后由 SQLite 自增。
//   Username — 登录名；全局唯一，不可为空。
//   PasswordHash — bcrypt 等密码哈希；JSON 序列化时省略。
//   Enabled — 是否允许 Web 登录与隧道握手；false 时拒绝认证。
//   MustChangePassword — 首次登录须改密；管理员创建时可置 true。
//   IsAdmin — Web 管理员；可访问管理 API，未必有隧道密钥。
//   PublicKey — WireGuard 风格公钥（Base64）；空表示纯 Web 账号（如 admin）。
//   PrivateKeyEnc — 私钥明文或 AES 密封密文；仅服务端解密下发，JSON 省略。
//   VPNIP — 分配的隧道内 IPv4；fixed 模式创建时写入，动态模式握手时分配。
//   AllowedIPs — 客户端分流 CIDR 列表；空时由 persist.ResolveAllowedIPs 回退服务端默认。
//   IPMode — 分配模式：fixed / dynamic_session / dynamic_lease（见 IPMode* 常量）。
//   IPLeaseSec — dynamic_lease 租约秒数；≤0 时读写默认 86400。
//   PolicyVer — 策略版本；变更 AllowedIPs 等时递增，握手推送给客户端。
//   CreatedAt — 记录创建时间（UTC 存库）。
//   UpdatedAt — 最近更新时间；密码、VPN 字段变更时刷新。
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

// AuditEntry 管理操作审计日志，对应 audit_logs 表一行。
//
// 字段：
//   ActorUserID — 操作者 users.id；系统动作可为 nil；DeleteUser 时可能置空保留记录。
//   Action — 动作标识（如 user.create、vpn.disable）；由 api/audit 包定义。
//   TargetType — 目标类型（user、session 等）；与 TargetID 成对解读。
//   TargetID — 目标主键；可为 nil 表示无具体对象。
//   ClientIP — 发起请求的客户端 IP；Web API 写入。
//   DetailJSON — 可选 JSON 附加字段（变更前后值等）；空串表示无。
//   CreatedAt — 入库时间（UTC）。
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

// ConnectionEvent 隧道连接上下线事件，对应 connection_events 表一行。
//
// 字段：
//   ID — 自增主键。
//   UserID — 关联 users.id；握手成功后写入。
//   EventType — 事件类型（如 connect、disconnect、auth_fail）；由 sessionmgr 定义。
//   RemoteAddr — 客户端 TCP 远端地址（host:port）。
//   DetailJSON — 可选 JSON 附加信息（错误码、策略版本等）；空串表示无。
//   CreatedAt — 事件入库时间。
type ConnectionEvent struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	EventType  string    `json:"event_type"`
	RemoteAddr string    `json:"remote_addr"`
	DetailJSON string    `json:"detail_json,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SessionStat 在线会话实时统计，对应 session_stats 表（每 user_id 一行）。
//
// 字段：
//   UserID — 关联 users.id；Upsert 主键。
//   ConnectedAt — 本次会话建立时间；断线后可为 nil。
//   LastHeartbeat — 最近一次心跳/数据活跃时间；超时判定依据。
//   RxBytes — 自连接以来下行字节累计（服务端视角收包）。
//   TxBytes — 自连接以来上行字节累计（服务端视角发包）。
//   ReconnectCount — 本会话周期内重连次数；新连接时可重置或累加。
//   RemoteAddr — 当前或最近远端地址；无连接时可为空串。
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

// Open 打开或创建 SQLite 数据库并应用 schema.sql（CREATE TABLE IF NOT EXISTS）。
//
// 参数：path — 数据库文件路径；父目录须已存在。
// 返回：*Store 就绪后可 CRUD；err 常见原因为路径不可写、schema 应用失败或 ping 超时。
// 副作用：启用 foreign_keys、WAL、busy_timeout=5s；max_open_conns=1。
// 并发：返回的 Store 可在多 goroutine 调用，SQLite 层串行化写入。
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
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// Close 关闭底层 SQLite 连接并释放文件句柄。
//
// 返回：err 为 sql.DB.Close 的错误；重复调用安全（由 database/sql 处理）。
// 副作用：之后所有 Store 方法将失败；调用方应在进程退出或服务 Stop 时调用。
func (s *Store) Close() error { return s.db.Close() }

// DB 暴露底层 *sql.DB，供调用方开启事务或执行 Store 未封装的 SQL。
//
// 返回：与 Open 时同一连接池（max_open_conns=1）；Close 后不可再用。
// 并发：事务边界由调用方管理；多 goroutine 共用时应自行串行化或使用 tx。
func (s *Store) DB() *sql.DB { return s.db }

// CountUsers 统计 users 表总行数（含禁用与纯 Web 账号）。
//
// 返回：n 为账号总数；err 为查询失败（库已关闭、磁盘错误等）。
// 副作用：只读；常用于初始化时判断是否需创建默认管理员。
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
	ips := marshalStringSlice(u.AllowedIPs)
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
		nullStr(u.VPNIP), ips, u.IPMode, u.IPLeaseSec, u.PolicyVer)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateVPNFields 更新账号隧道字段（策略/IP 模式等）。
func (s *Store) UpdateVPNFields(id int64, vpnIP string, allowedIPs []string, ipMode string, ipLeaseSec, policyVer int) error {
	ips := marshalStringSlice(allowedIPs)
	_, err := s.db.Exec(`UPDATE users SET vpn_ip=?, allowed_ips=?, ip_mode=?, ip_lease_sec=?, policy_ver=?, updated_at=datetime('now') WHERE id=?`,
		nullStr(vpnIP), ips, ipMode, ipLeaseSec, policyVer, id)
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

// GetUserByUsername 按登录名查询用户（Web 登录与隧道身份共用）。
//
// 参数：username — 非空、与 users.username 精确匹配。
// 返回：*User 含密码哈希等敏感字段；未找到时 err 为 sql.ErrNoRows。
// 副作用：只读。
func (s *Store) GetUserByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE username=?`, username)
	return scanUser(row)
}

// GetUserByID 按主键查询用户。
//
// 参数：id — users.id；须为正整数。
// 返回：*User；未找到时 err 为 sql.ErrNoRows。
// 副作用：只读；IncrementPolicyVer 等内部方法也会调用。
func (s *Store) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE id=?`, id)
	return scanUser(row)
}

// GetUserByPublicKey 隧道握手：按公钥查找 VPN 账号。
func (s *Store) GetUserByPublicKey(pub string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE public_key=?`, pub)
	return scanUser(row)
}

// ListUsers 列出全部用户，按 id 升序。
//
// 返回：[]User 含 Web 与 VPN 账号；无用户时为空切片非 nil；err 为查询失败。
// 副作用：只读；管理 API 用户列表与导出功能使用。
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

// UpdateUserPassword 更新用户密码哈希并可选清除「须改密」标记。
//
// 参数：hash — 已哈希密码（如 bcrypt）；clearMustChange 为 true 时 must_change_password=0。
// 返回：err 为 UPDATE 失败或 id 不存在（影响行数 0 仍返回 nil）。
// 副作用：刷新 updated_at。
func (s *Store) UpdateUserPassword(id int64, hash string, clearMustChange bool) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=?, must_change_password=?, updated_at=datetime('now') WHERE id=?`,
		hash, boolToInt(!clearMustChange), id)
	return err
}

// SetUserEnabled 启用或禁用账号（Web 登录与隧道握手均受 enabled 约束）。
//
// 参数：enabled — false 时拒绝认证，不断开已有 TCP（由 sessionmgr 处理）。
// 返回：err 为 UPDATE 失败。
// 副作用：刷新 updated_at；禁用后新握手失败。
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

// InsertAuditLog 追加一条审计记录到 audit_logs。
//
// 参数：e — Action/TargetType 等由调用方填好；ActorUserID、TargetID 可 nil。
// 返回：err 为 INSERT 失败（外键、磁盘等）。
// 副作用：写库；不可更新或删除（DeleteUser 仅置空 actor_user_id）。
func (s *Store) InsertAuditLog(e AuditEntry) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(actor_user_id, action, target_type, target_id, client_ip, detail_json) VALUES(?,?,?,?,?,?)`,
		e.ActorUserID, e.Action, e.TargetType, e.TargetID, e.ClientIP, e.DetailJSON)
	return err
}

// ListAuditLogs 分页查询审计日志，按 id 降序（最新在前）。
//
// 参数：limit — 每页条数；offset — 跳过条数（管理 API 分页）。
// 返回：[]AuditEntry；err 为查询失败。
// 副作用：只读。
func (s *Store) ListAuditLogs(limit, offset int) ([]AuditEntry, error) {
	rows, err := s.db.Query(`SELECT id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}

// InsertConnectionEvent 写入隧道连接/断线/认证失败等事件。
//
// 参数：userID — 关联 users.id；eventType — 如 connect、disconnect；detail 为 JSON 或空。
// 返回：err 为 INSERT 失败。
// 副作用：写 connection_events；供仪表盘与 ListRecentConnectionEvents 展示。
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
	return scanConnectionEventRows(rows)
}

// UpsertSessionStat 插入或更新在线会话统计（每 user_id 一行）。
//
// 参数：st — UserID 必填；ConnectedAt/LastHeartbeat 可为 nil 表示未知。
// 返回：err 为 UPSERT 失败。
// 副作用：写 session_stats；sessionmgr 心跳与断线时调用。
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

// GetSessionStat 读取指定用户的会话统计行。
//
// 参数：userID — users.id。
// 返回：*SessionStat；无记录时 err 为 sql.ErrNoRows。
// 副作用：只读；管理 API 展示在线状态与流量时使用。
func (s *Store) GetSessionStat(userID int64) (*SessionStat, error) {
	row := s.db.QueryRow(`SELECT user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr FROM session_stats WHERE user_id=?`, userID)
	var st SessionStat
	var connAt, hb, remote sql.NullString
	if err := row.Scan(&st.UserID, &connAt, &hb, &st.RxBytes, &st.TxBytes, &st.ReconnectCount, &remote); err != nil {
		return nil, err
	}
	st.ConnectedAt = parseSQLiteTimePtr(connAt)
	st.LastHeartbeat = parseSQLiteTimePtr(hb)
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
	unmarshalAllowedIPs(ipsJSON, &u.AllowedIPs)
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
	u.CreatedAt = parseSQLiteTime(created)
	u.UpdatedAt = parseSQLiteTime(updated)
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
	return formatSQLiteTime(*t)
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
