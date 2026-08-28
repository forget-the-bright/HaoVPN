package auth

import (
	"time"

	"haovpn/internal/logger"
)

// isLocked 判断 clientIP 是否处于登录失败锁定期内。
//
// 锁定过期后自动清除条目；未达 maxAttempts 时 Failures 累计但不锁定。
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

// recordFailure 记录一次登录失败；达 maxAttempts 时设置 LockedUntil。
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

// clearFailures 登录成功后清除该 IP 的失败计数与锁定状态。
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
