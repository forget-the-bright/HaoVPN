package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"haovpn/internal/persist"
)

// createSession 为已认证用户签发新 Web 会话 token 与 CSRF。
//
// 参数：u — 须非 nil；写入 sessions map 并设置 ExpiresAt=now+sessionTTL。
// 返回：sessionToken、csrfToken；randomToken 失败时 err 非 nil。
func (s *Service) createSession(u *persist.User) (string, string, error) {
	token, err := randomToken()
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now()
	s.sessionsMu.Lock()
	s.sessions[token] = SessionEntry{
		UserID:            u.ID,
		Username:          u.Username,
		ExpiresAt:         now.Add(s.sessionTTL),
		AbsoluteExpiresAt: now.Add(2 * s.sessionTTL), // 绝对上限：滑动续期不可超过创建时刻 + 2×TTL
		CSRFToken:         csrf,
	}
	s.sessionsMu.Unlock()
	return token, csrf, nil
}

// ValidateSession 校验 session token 是否仍有效且未过期。
//
// 参数：token 为 Login 返回的会话串；空串视为无效。
// 返回：有效时 SessionEntry 与 true；不存在或过期时零值与 false（并顺带删除过期条目）。
// 并发：持写锁以便 miss 时 prune；可与 Logout 并发。
func (s *Service) ValidateSession(token string) (SessionEntry, bool) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	se, ok := s.sessions[token]
	if !ok {
		return SessionEntry{}, false
	}
	now := time.Now()
	if now.After(se.ExpiresAt) || (!se.AbsoluteExpiresAt.IsZero() && now.After(se.AbsoluteExpiresAt)) {
		delete(s.sessions, token)
		return SessionEntry{}, false
	}
	return se, true
}

// TouchSession 滑动续期：将 ExpiresAt 延长至 now+TTL，但不超过 AbsoluteExpiresAt。
//
// 鉴权成功路径调用；token 无效时 no-op。
func (s *Service) TouchSession(token string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	se, ok := s.sessions[token]
	if !ok {
		return
	}
	now := time.Now()
	if !se.AbsoluteExpiresAt.IsZero() && now.After(se.AbsoluteExpiresAt) {
		delete(s.sessions, token)
		return
	}
	next := now.Add(s.sessionTTL)
	if !se.AbsoluteExpiresAt.IsZero() && next.After(se.AbsoluteExpiresAt) {
		next = se.AbsoluteExpiresAt
	}
	se.ExpiresAt = next
	s.sessions[token] = se
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

// LogoutAllForUser 吊销某用户的全部 Web 会话（改密/重置/禁用后调用，防盗用 Cookie 继续有效）。
//
// 参数：userID — persist.User 主键。
// 返回：被删除的会话数。
func (s *Service) LogoutAllForUser(userID int64) int {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	n := 0
	for tok, se := range s.sessions {
		if se.UserID == userID {
			delete(s.sessions, tok)
			n++
		}
	}
	return n
}

// PruneExpiredSessions 删除已过期的内存会话，防止长跑进程泄漏。
//
// 返回：删除条数；登录成功路径与后台 ticker 均可调用。
func (s *Service) PruneExpiredSessions() int {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	n := 0
	now := time.Now()
	for tok, se := range s.sessions {
		expired := now.After(se.ExpiresAt)
		if !se.AbsoluteExpiresAt.IsZero() && now.After(se.AbsoluteExpiresAt) {
			expired = true
		}
		if expired {
			delete(s.sessions, tok)
			n++
		}
	}
	return n
}

// PruneExpiredLockouts 清理已过锁定窗口的 IP 条目，防止喷洒导致 map 无限增长。
func (s *Service) PruneExpiredLockouts() int {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	n := 0
	now := time.Now()
	for _, m := range []map[string]lockoutEntry{s.webLockouts, s.tunnelLockouts} {
		for ip, e := range m {
			if !e.LockedUntil.IsZero() && now.After(e.LockedUntil) {
				delete(m, ip)
				n++
			}
		}
	}
	return n
}

// ValidateCSRF 校验写请求是否携带与 session 匹配的 CSRF token。
//
// 参数：sessionToken 为 Cookie/Header 中的会话；csrf 为表单或 Header 中的 CSRF 值。
// 返回：会话有效且 CSRF 一致时为 true；会话无效或 token 不匹配为 false。
// 副作用：无；内部调用 ValidateSession；比较使用常量时间以防时序侧信道。
func (s *Service) ValidateCSRF(sessionToken, csrf string) bool {
	se, ok := s.ValidateSession(sessionToken)
	if !ok {
		return false
	}
	a := []byte(se.CSRFToken)
	b := []byte(csrf)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
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
// randomToken 生成 32 字节随机 hex 串，供 session/CSRF 使用。
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
