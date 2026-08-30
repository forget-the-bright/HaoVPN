package config

import (
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// clampSendQueueLogged 钳制发送队列深度；越界时 Warn 并返回合法值。
func clampSendQueueLogged(field string, n int) int {
	clamped, changed := netutil.ClampSendQueueSize(n)
	if changed {
		logger.Warn("%s=%d 已钳制为 %d（允许 %d～%d，≤0 用默认 %d）",
			field, n, clamped, netutil.MinSendQueueSize, netutil.MaxSendQueueSize, netutil.DefaultSendQueueSize)
	}
	return clamped
}
