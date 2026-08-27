// Package auth handles user authentication and sessions.
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

// Service manages authentication.
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

// SessionEntry 表示一个已登录的 Web 会话。
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

// New creates an auth service.
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

// HashPassword returns bcrypt hash.
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

// CheckPassword verifies password against hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// EnsureAdmin 无用户时创建默认 admin；syncFromConfig 为 true 时按 yaml 同步 admin 密码（开发/home）。
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

// Login 管理 Web 登录：仅 is_admin 账号可创建会话。
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

// VerifyTunnelLogin 隧道握手鉴权：校验账号密码与锁定，不创建 Web 会话。
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

// ValidateSession returns user ID if session is valid.
func (s *Service) ValidateSession(token string) (SessionEntry, bool) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	se, ok := s.sessions[token]
	if !ok || time.Now().After(se.ExpiresAt) {
		return SessionEntry{}, false
	}
	return se, true
}

// Logout removes a session.
func (s *Service) Logout(token string) {
	s.sessionsMu.Lock()
	delete(s.sessions, token)
	s.sessionsMu.Unlock()
}

// ValidateCSRF checks CSRF token for session.
func (s *Service) ValidateCSRF(sessionToken, csrf string) bool {
	se, ok := s.ValidateSession(sessionToken)
	if !ok {
		return false
	}
	return se.CSRFToken == csrf
}

// GetCSRF returns CSRF token for session.
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
