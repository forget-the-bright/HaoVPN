package auth

import (
	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// EnsureAdmin 保证至少存在一个管理员账号，并按配置决定是否同步密码。
//
// 参数：username/password 为配置中的 admin 凭据；syncFromConfig true 时允许用 yaml 覆盖已有 admin 哈希并清除须改密标记。
// 返回：DB 读写或 HashPassword 失败时 err；用户不存在且 sync 时仅 Warn 跳过。
// 副作用：可能 CreateUser、UpdateUserPassword；写 Info/Warn 日志。
func (s *Service) EnsureAdmin(username, password string, syncFromConfig bool) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
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
// 返回：session token、User 指针；凭据错误、锁定或非 admin 时 err 非 nil（哨兵见 errors.go）。
// 副作用：成功时写入 sessions；失败时可能累加 webLockouts 并打 Warn 日志。
func (s *Service) Login(username, password, clientIP string) (string, *persist.User, error) {
	u, err := s.verifyCredentials(lockoutWeb, username, password, clientIP)
	if err != nil {
		return "", nil, err
	}
	if !u.IsAdmin {
		// 模糊文案：不泄露「存在但非管理员」，与错密同形
		logger.Warn("Web 登录拒绝: 非管理账号 user=%s ip=%s", username, clientIP)
		s.recordFailure(lockoutWeb, clientIP)
		return "", nil, ErrBadCredentials
	}
	token, csrf, err := s.createSession(u)
	if err != nil {
		return "", nil, err
	}
	_ = csrf
	logger.Info("user logged in: %s from %s", username, clientIP)
	return token, u, nil
}

// verifyCredentials 校验用户名密码；realm 决定写入哪张锁定表。
//
// 禁用账号对外统一返回 ErrBadCredentials（防枚举）；内部仍打 Warn。
func (s *Service) verifyCredentials(realm lockoutRealm, username, password, clientIP string) (*persist.User, error) {
	if s.isLocked(realm, clientIP) {
		logger.Warn("登录失败: IP 已锁定 realm=%d ip=%s", realm, clientIP)
		return nil, ErrLoginLocked
	}
	u, err := s.store.GetUserByUsername(username)
	if err != nil {
		s.recordFailure(realm, clientIP)
		logger.Warn("登录失败: 用户不存在 username=%s ip=%s", username, clientIP)
		return nil, ErrBadCredentials
	}
	if !u.Enabled {
		s.recordFailure(realm, clientIP)
		logger.Warn("登录失败: 账号已禁用 user=%s ip=%s", username, clientIP)
		return nil, ErrBadCredentials
	}
	if !CheckPassword(u.PasswordHash, password) {
		s.recordFailure(realm, clientIP)
		logger.Warn("登录失败: 密码错误 user=%s ip=%s failures=%d", username, clientIP, s.failureCount(realm, clientIP))
		return nil, ErrBadCredentials
	}
	s.clearFailures(realm, clientIP)
	return u, nil
}
