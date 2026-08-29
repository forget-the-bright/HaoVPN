package auth

import (
	"sync"
	"time"

	"haovpn/internal/persist"
	"haovpn/internal/timeutil"
)

const bcryptCost = 12

// Service 服务端 Web 与隧道共用的鉴权与会话管理器。
//
// 字段：
//   store — 持久化用户库；所有账号读写经此访问。
//   sessions — 内存 Web 会话表（token → SessionEntry）；进程重启后清空。
//   webLockouts / tunnelLockouts — 按 clientIP 分表累计失败（Web 与隧道互不影响）。
//   maxAttempts / lockoutSec / sessionTTL — 来自配置；≤0 时 New 使用内置默认值。
//
// 线程安全：sessions 用 RWMutex，lockouts 用 Mutex；可并发校验会话与登录。
type Service struct {
	store          *persist.Store
	sessions       map[string]SessionEntry
	sessionsMu     sync.RWMutex
	webLockouts    map[string]lockoutEntry
	tunnelLockouts map[string]lockoutEntry
	lockoutsMu     sync.Mutex
	maxAttempts    int
	lockoutSec     int
	sessionTTL     time.Duration
}

// SessionEntry 表示一条有效的 Web 管理端登录会话。
//
// 字段：
//   UserID — 对应 persist.User 主键。
//   Username — 登录名快照；便于日志与 CSRF 校验，不随 DB 改名自动更新。
//   ExpiresAt — 会话过期 UTC 时间；ValidateSession 会拒绝已过期条目。
//   CSRFToken — 与该 session token 配对的 CSRF 随机串；写操作须 ValidateCSRF。
type SessionEntry struct {
	UserID    int64
	Username  string
	ExpiresAt time.Time
	CSRFToken string
}

type lockoutEntry struct {
	Failures    int
	LockedUntil time.Time
}

// New 构造鉴权服务并初始化内存会话与锁定表。
//
// 参数：store 非空；maxAttempts/lockoutSec/sessionTTLSec ≤0 时分别默认为 5、900 秒、28800 秒。
// 返回：就绪的 *Service；不访问数据库直至 EnsureAdmin/Login 等被调用。
// 副作用：无；仅分配内存 map。
func New(store *persist.Store, maxAttempts, lockoutSec, sessionTTLSec int) *Service {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if lockoutSec <= 0 {
		lockoutSec = 900
	}
	if sessionTTLSec <= 0 {
		sessionTTLSec = 28800
	}
	return &Service{
		store:          store,
		sessions:       map[string]SessionEntry{},
		webLockouts:    map[string]lockoutEntry{},
		tunnelLockouts: map[string]lockoutEntry{},
		maxAttempts:    maxAttempts,
		lockoutSec:     lockoutSec,
		sessionTTL:     timeutil.Seconds(sessionTTLSec),
	}
}
