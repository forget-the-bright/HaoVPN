package winnet

import "net"

// DefaultRouteScrubMode 控制清 TUN 默认路由时是否允许 PowerShell fallback（跨平台类型定义）。
type DefaultRouteScrubMode int

const (
	// ScrubDefaultRouteFast 仅查表 + IP Helper，不 PS。
	ScrubDefaultRouteFast DefaultRouteScrubMode = iota
	// ScrubDefaultRouteLate ICS 后纵深；iphlp 失败时 PS fallback。
	ScrubDefaultRouteLate
)

// IsIPv4DefaultRouteOnIf 判断路由表行是否为指定 ifIndex 上的 IPv4 默认路由（0.0.0.0/0）。
//
// 纯函数，供 hasDefaultRouteOnInterface 与单测共用；与平台无关。
func IsIPv4DefaultRouteOnIf(ifIndex int, rowIfIndex int, dest net.IP, prefixLen int) bool {
	if ifIndex <= 0 || rowIfIndex != ifIndex {
		return false
	}
	d := dest.To4()
	if d == nil {
		return false
	}
	return prefixLen == 0 && d.Equal(net.IPv4zero)
}
