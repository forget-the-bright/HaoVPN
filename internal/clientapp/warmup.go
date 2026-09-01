package clientapp

import (
	"haovpn/internal/logger"
	"haovpn/internal/safeutil"
	"haovpn/internal/tun"
)

// StartWarmupAsync 后台预热 TUN（CLI/GUI 共用；与 Start 重叠，勿 Wait 后再拨号）。
func StartWarmupAsync(tunName string) {
	safeutil.GoSafe("client-tun-warmup", func() {
		logger.Info("client_bootstrap warmup=begin name=%s", tunName)
		if err := warmupTun(tunName); err != nil {
			logger.Warn("client_bootstrap warmup=fail name=%s err=%v（Start 时仍会 Open/Create）", tunName, err)
			return
		}
		logger.Info("client_bootstrap warmup=done name=%s", tunName)
	})
}

// warmupTun 在登录/拨号前预热名为 name 的 TUN/Wintun 适配器（Open 或 Create）。
//
// 为何放在 clientapp 而非 GUI 直接调 tun：
//   - clientgui 只应编排 UI，不得依赖数据面叶子包（分层：GUI → clientapp → tun）；
//   - 预热与 Open 复用同一适配器句柄（reuse from_warmup），须与引擎 Open 路径同属一层语义。
//
// 参数：name — 通常来自 client.yaml tun.name（空则由 tun 包套用默认名）。
// 返回：创建/打开失败时的错误；成功时后台已持有句柄，供后续 Open 复用。
// 关联：StartWarmupAsync；勿 Wait 完成后再 auto_connect（会空等数秒）。
func warmupTun(name string) error {
	return tun.WarmupAdapter(name)
}
