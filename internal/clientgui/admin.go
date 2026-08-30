package clientgui

import "haovpn/internal/platform"

// requireAdmin 校验当前进程是否具备 OS 管理员权限；不足时写入提示并返回 false。
//
// 参数：onDeny — 非空时调用（写入 UI 错误或托盘日志）；msg 为中文原因。
// 为何集中：login / tray_config 多处同一门禁，避免散落 IsAdmin 样板。
// 注意：这是 OS 提权（TUN/路由），与 Web RBAC User.IsAdmin 无关。
func (u *uiApp) requireAdmin(msg string, onDeny func(string)) bool {
	if platform.IsAdmin() {
		return true
	}
	if onDeny != nil {
		onDeny(msg)
	}
	return false
}
