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
