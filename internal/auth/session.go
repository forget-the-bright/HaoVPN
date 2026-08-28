package auth

import (
	"crypto/rand"
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
// randomToken 生成 32 字节随机 hex 串，供 session/CSRF 使用。
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
