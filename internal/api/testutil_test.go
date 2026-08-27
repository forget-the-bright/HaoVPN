package api_test

import (
	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/vpnaccount"
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

// testVPNService 构造 api.NewServer 所需的 vpnaccount.Service（测试用，无 session 回调）。
func testVPNService(store *persist.Store, pool *ippool.Pool, cfg *config.ServerConfig) *vpnaccount.Service {
	return &vpnaccount.Service{Store: store, Pool: pool, Cfg: cfg}
}
