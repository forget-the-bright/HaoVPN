package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

const bcryptCost = 12

// Service 服务端 Web 与隧道共用的鉴权与会话管理器。
//
// 字段：
//   store — 持久化用户库；所有账号读写经此访问。
//   sessions — 内存 Web 会话表（token → SessionEntry）；进程重启后清空。
//   lockouts — 按 clientIP 累计失败次数与锁定截止时间。
//   maxAttempts / lockoutSec / sessionTTL — 来自配置；≤0 时 New 使用内置默认值。
//
// 线程安全：sessions 用 RWMutex，lockouts 用 Mutex；可并发校验会话与登录。
type Service struct {
	store      *persist.Store
	sessions   map[string]SessionEntry
	sessionsMu sync.RWMutex
	lockouts   map[string]lockoutEntry
	lockoutsMu sync.Mutex
	maxAttempts int
	lockoutSec  int
	sessionTTL  time.Duration
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
	Failures  int
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
		store:       store,
		sessions:    map[string]SessionEntry{},
		lockouts:    map[string]lockoutEntry{},
		maxAttempts: maxAttempts,
		lockoutSec:  lockoutSec,
		sessionTTL:  time.Duration(sessionTTLSec) * time.Second,
	}
}

// HashPassword 使用 bcrypt（cost=12）生成密码哈希，供入库或改密。
//
// 参数：password 长度须 ≥8，否则返回错误。
// 返回：可存入 PasswordHash 的字符串；err 为 bcrypt 或校验失败。
// 副作用：无；纯函数。
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验明文密码是否与 bcrypt 哈希匹配。
//
// 参数：hash 为库中 PasswordHash；password 为本次输入。
// 返回：匹配 true，否则 false；不区分「用户不存在」与「密码错误」（由上层统一文案）。
// 副作用：无；可并发调用。
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EnsureAdmin 保证至少存在一个管理员账号，并按配置决定是否同步密码。
//
// 参数：username/password 为配置中的 admin 凭据；syncFromConfig true 时允许用 yaml 覆盖已有 admin 哈希并清除须改密标记。
// 返回：DB 读写或 HashPassword 失败时 err；用户不存在且 sync 时仅 Warn 跳过。
// 副作用：可能 CreateUser、UpdateUserPassword；写 Info/Warn 日志。
func (s *Service) EnsureAdmin(username, password string, syncFromConfig bool) error {
	n, err := s.store.CountUsers()
	if err != nil {
		return err
	}
	if n == 0 {
		hash, err := HashPassword(password)
		if err != nil {
			return err
		}
		// syncFromConfig 表示开发/home 用 yaml 密码，首启也不应强制改密
		mustChange := !syncFromConfig
		_, err = s.store.CreateUser(username, hash, mustChange)
		if err != nil {
			return err
		}
		if mustChange {
			logger.Info("default admin user created: %s (must change password on first login)", username)
		} else {
			logger.Info("default admin user created: %s (sync_password_from_config=true)", username)
		}
		return nil
	}
	if !syncFromConfig || password == "" {
		return nil
	}
	u, err := s.store.GetUserByUsername(username)
	if err != nil {
		logger.Warn("sync_password_from_config: 用户 %q 不存在，跳过", username)
		return nil
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	// sync 场景须清除「首次改密」标记：第三参为 clearMustChange（true=不再强制改密），
	// 不可传 u.MustChangePassword（语义相反，会导致每次重启又把 must_change 置 1）。
	if err := s.store.UpdateUserPassword(u.ID, hash, true); err != nil {
		return err
	}
	logger.Info("admin 密码已从配置文件同步（sync_password_from_config=true，已清除须改密标记）")
	return nil
}

// Login 处理 Web 管理端登录并签发 session token。
//
// 参数：username/password 为用户凭据；clientIP 用于失败锁定与审计。
// 返回：session token、User 指针；非 admin、凭据错误或锁定时 err 非 nil。
// 副作用：成功时写入 sessions；失败时可能累加 lockouts 并打 Warn 日志。
func (s *Service) Login(username, password, clientIP string) (string, *persist.User, error) {
	u, err := s.verifyCredentials(username, password, clientIP)
	if err != nil {
		return "", nil, err
	}
	if !u.IsAdmin {
		logger.Warn("Web 登录拒绝: 非管理账号 user=%s ip=%s", username, clientIP)
		return "", nil, errors.New("非管理账号，无法登录 Web")
	}
	token, csrf, err := s.createSession(u)
	if err != nil {
		return "", nil, err
	}
	_ = csrf
	logger.Info("user logged in: %s from %s", username, clientIP)
	return token, u, nil
}

// VerifyTunnelLogin 隧道握手阶段的账号密码校验，不创建 Web 会话。
//
// 参数：username/password/clientIP 同 Login；须已开通 VPN 密钥且非「须改密」状态。
// 返回：通过时 *persist.User；须改密、无密钥、锁定或凭据错误时 err 非 nil。
// 副作用：失败时可能累加 lockouts；不写 sessions。
func (s *Service) VerifyTunnelLogin(username, password, clientIP string) (*persist.User, error) {
	u, err := s.verifyCredentials(username, password, clientIP)
	if err != nil {
		return nil, err
	}
	if u.MustChangePassword {
		logger.Warn("隧道登录拒绝: 须先修改密码 user=%s ip=%s", username, clientIP)
		return nil, errors.New("须先修改密码后再连接 VPN（请用 Web 管理端或联系管理员）")
	}
	if u.PublicKey == "" || u.PrivateKeyEnc == "" {
		return nil, errors.New("账号未开通 VPN（无密钥）")
	}
	return u, nil
}

func (s *Service) verifyCredentials(username, password, clientIP string) (*persist.User, error) {
	if s.isLocked(clientIP) {
		logger.Warn("登录失败: IP 已锁定 ip=%s", clientIP)
		return nil, errors.New("登录失败次数过多，请稍后再试")
	}
	u, err := s.store.GetUserByUsername(username)
	if err != nil {
		s.recordFailure(clientIP)
		logger.Warn("登录失败: 用户不存在 username=%s ip=%s", username, clientIP)
		return nil, errors.New("用户名或密码错误")
	}
	if !u.Enabled {
		s.recordFailure(clientIP)
		logger.Warn("登录失败: 账号已禁用 user=%s ip=%s", username, clientIP)
		return nil, errors.New("账号已禁用")
	}
	if !CheckPassword(u.PasswordHash, password) {
		s.recordFailure(clientIP)
		logger.Warn("登录失败: 密码错误 user=%s ip=%s failures=%d", username, clientIP, s.failureCount(clientIP))
		return nil, errors.New("用户名或密码错误")
	}
	s.clearFailures(clientIP)
	return u, nil
}

func (s *Service) createSession(u *persist.User) (string, string, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", err
	}
	s.sessionsMu.Lock()
	s.sessions[token] = SessionEntry{
		UserID:    u.ID,
		Username:  u.Username,
		ExpiresAt: time.Now().Add(s.sessionTTL),
		CSRFToken: csrf,
	}
	s.sessionsMu.Unlock()
	return token, csrf, nil
}

// ValidateSession 校验 session token 是否仍有效且未过期。
//
// 参数：token 为 Login 返回的会话串；空串视为无效。
// 返回：有效时 SessionEntry 与 true；不存在或过期时零值与 false。
// 并发：持 RLock 只读；可与 Logout 并发（可能读到已删除 token）。
func (s *Service) ValidateSession(token string) (SessionEntry, bool) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	se, ok := s.sessions[token]
	if !ok || time.Now().After(se.ExpiresAt) {
		return SessionEntry{}, false
	}
	return se, true
}

// Logout 使指定 Web 会话 token 立即失效。
//
// 参数：token 可为任意串；不存在时静默 no-op。
// 返回：无。
// 副作用：从 sessions map 删除条目；持写锁。
func (s *Service) Logout(token string) {
	s.sessionsMu.Lock()
	delete(s.sessions, token)
	s.sessionsMu.Unlock()
}

// ValidateCSRF 校验写请求是否携带与 session 匹配的 CSRF token。
//
// 参数：sessionToken 为 Cookie/Header 中的会话；csrf 为表单或 Header 中的 CSRF 值。
// 返回：会话有效且 CSRF 一致时为 true；会话无效或 token 不匹配为 false。
// 副作用：无；内部调用 ValidateSession。
func (s *Service) ValidateCSRF(sessionToken, csrf string) bool {
	se, ok := s.ValidateSession(sessionToken)
	if !ok {
		return false
	}
	return se.CSRFToken == csrf
}

// GetCSRF 返回有效会话对应的 CSRF token，供模板或 API 下发给前端。
//
// 参数：sessionToken 须为当前有效会话。
// 返回：CSRF 字符串；会话无效时空串。
// 副作用：无。
func (s *Service) GetCSRF(sessionToken string) string {
	se, ok := s.ValidateSession(sessionToken)
	if !ok {
		return ""
	}
	return se.CSRFToken
}

func (s *Service) isLocked(ip string) bool {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	e, ok := s.lockouts[ip]
	if !ok {
		return false
	}
	// 仅当已触发锁定时检查 LockedUntil；未锁定时保留 Failures 累计
	if e.LockedUntil.IsZero() {
		return false
	}
	if time.Now().Before(e.LockedUntil) {
		return true
	}
	delete(s.lockouts, ip)
	return false
}

func (s *Service) recordFailure(ip string) {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	e := s.lockouts[ip]
	e.Failures++
	if e.Failures >= s.maxAttempts {
		e.LockedUntil = time.Now().Add(time.Duration(s.lockoutSec) * time.Second)
		logger.Warn("login locked for IP %s after %d failures", ip, e.Failures)
	}
	s.lockouts[ip] = e
}

func (s *Service) clearFailures(ip string) {
	s.lockoutsMu.Lock()
	delete(s.lockouts, ip)
	s.lockoutsMu.Unlock()
}

// failureCount 返回当前 IP 累计失败次数（仅用于日志，不触发锁逻辑）。
func (s *Service) failureCount(ip string) int {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	if e, ok := s.lockouts[ip]; ok {
		return e.Failures
	}
	return 0
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
