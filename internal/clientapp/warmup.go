package clientapp

import "haovpn/internal/tun"

// WarmupTun 在登录/拨号前预热名为 name 的 TUN/Wintun 适配器（Open 或 Create）。
//
// 为何放在 clientapp 而非 GUI 直接调 tun：
//   - clientgui 只应编排 UI，不得依赖数据面叶子包（分层：GUI → clientapp → tun）；
//   - 预热与 Open 复用同一适配器句柄（reuse from_warmup），须与引擎 Open 路径同属一层语义。
//
// 参数：name — 通常来自 client.yaml tun.name（空则由 tun 包套用默认名）。
// 返回：创建/打开失败时的错误；成功时后台已持有句柄，供后续 Open 复用。
// 关联：clientgui/run.go 启动时 GoSafe 调用；勿 Wait 完成后再 auto_connect（会空等数秒）。
func WarmupTun(name string) error {
	return tun.WarmupAdapter(name)
}
