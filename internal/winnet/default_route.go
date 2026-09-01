package winnet

import "net"

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
