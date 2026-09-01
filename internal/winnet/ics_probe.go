package winnet

import "haovpn/internal/netutil"

// InterfaceHasICSPrivate 判断指定 ifIndex 上是否已有 ICS 私网单播（192.168.137.*）。
//
// 用途：配 VPN IP 时决定 KeepICS+/24；PreferVPN light 判断是否还需完整 PS 回退。
// 为何集中：避免 assign_ip / Prefer / soft-replace 各写一遍 ListUnicast+IPv4IsICSPrivate。
// 失败（列地址失败）返回 false（偏保守：走非 KeepICS / 完整 Prefer，不误判有活 ICS）。
//
// 关联：HasICSResidue（按 TUN 名探测，可走 cache）；本函数按 ifIndex，不碰 PowerShell。
func InterfaceHasICSPrivate(ifIndex int) bool {
	if ifIndex <= 0 {
		return false
	}
	ents, err := ListUnicastIPv4OnIfIndex(ifIndex)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if netutil.IPv4IsICSPrivate(e.IP) {
			return true
		}
	}
	return false
}
