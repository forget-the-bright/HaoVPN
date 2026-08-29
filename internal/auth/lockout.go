package auth

import (
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/timeutil"
)

// lockoutRealm 区分 Web 与隧道锁定表，避免 VPN 喷洒锁死管理口（或反之）。
type lockoutRealm int

const (
	lockoutWeb lockoutRealm = iota
	lockoutTunnel
)

func (s *Service) lockoutMap(realm lockoutRealm) map[string]lockoutEntry {
	if realm == lockoutTunnel {
		return s.tunnelLockouts
	}
	return s.webLockouts
}

// isLocked 判断 clientIP 是否处于指定 realm 的登录失败锁定期内。
func (s *Service) isLocked(realm lockoutRealm, ip string) bool {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	m := s.lockoutMap(realm)
	e, ok := m[ip]
	if !ok {
		return false
	}
	if e.LockedUntil.IsZero() {
		return false
	}
	if time.Now().Before(e.LockedUntil) {
		return true
	}
	delete(m, ip)
	return false
}

// recordFailure 记录一次登录失败；达 maxAttempts 时设置 LockedUntil。
func (s *Service) recordFailure(realm lockoutRealm, ip string) {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	m := s.lockoutMap(realm)
	e := m[ip]
	e.Failures++
	if e.Failures >= s.maxAttempts {
		e.LockedUntil = time.Now().Add(timeutil.Seconds(s.lockoutSec))
		logger.Warn("login locked realm=%d for IP %s after %d failures", realm, ip, e.Failures)
	}
	m[ip] = e
}

// clearFailures 登录成功后清除该 IP 在指定 realm 的失败计数与锁定状态。
func (s *Service) clearFailures(realm lockoutRealm, ip string) {
	s.lockoutsMu.Lock()
	delete(s.lockoutMap(realm), ip)
	s.lockoutsMu.Unlock()
}

// failureCount 返回当前 IP 在指定 realm 的累计失败次数（仅用于日志）。
func (s *Service) failureCount(realm lockoutRealm, ip string) int {
	s.lockoutsMu.Lock()
	defer s.lockoutsMu.Unlock()
	if e, ok := s.lockoutMap(realm)[ip]; ok {
		return e.Failures
	}
	return 0
}
