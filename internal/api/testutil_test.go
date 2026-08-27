package api_test

import (
	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// ensureTestAdmin 创建测试用 admin 并清除须改密标记（避免 must_change 阻塞 API 测试）。
func ensureTestAdmin(store *persist.Store, authSvc *auth.Service, username, password string) error {
	if err := authSvc.EnsureAdmin(username, password, false); err != nil {
		return err
	}
	u, err := store.GetUserByUsername(username)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return store.UpdateUserPassword(u.ID, hash, true)
}
