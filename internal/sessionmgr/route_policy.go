package sessionmgr

import (
	"net"
	"time"

	"haovpn/internal/netutil"
)

// route_policy.go：入站源/目的校验与横向访问策略（无 I/O）。

// sourceIPAllowed 入站源地址是否合法：本账号 VPN IP，或 via 出口注册网段内（LAN 回程）。
func sourceIPAllowed(ps *AccountSession, src net.IP) bool {
	if ps == nil || src == nil {
		return false
	}
	srcStr := src.String()
	if ps.VPNIP != "" && srcStr == ps.VPNIP {
		return true
	}
	return sourceFromExitLAN(ps, src)
}

// sourceFromExitLAN 源 IP 是否落在本会话 via 广告网段（不含本账号 VPN IP）。
func sourceFromExitLAN(ps *AccountSession, src net.IP) bool {
	if ps == nil || src == nil {
		return false
	}
	for _, n := range ps.ExitLANs {
		if n != nil && n.Contains(src) {
			return true
		}
	}
	return false
}

// shouldWarnSpoof 伪造源 WARN 限流：每会话至少间隔 10s 打一条。
func shouldWarnSpoof(ps *AccountSession) bool {
	if ps == nil {
		return true
	}
	now := time.Now().UnixNano()
	last := ps.lastSpoofWarn.Load()
	if now-last < int64(10*time.Second) {
		return false
	}
	return ps.lastSpoofWarn.CompareAndSwap(last, now)
}

// isNoiseSourceIP 常见无害噪声源（DHCP 0.0.0.0 等）。
func isNoiseSourceIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	return false
}

// dstAllowed 判断目的 IP 是否在该会话 VPN IP 或 AllowedIPs 网段内。
func (m *Manager) dstAllowed(ps *AccountSession, dst net.IP) bool {
	if ps.VPNIP != "" && dst.String() == ps.VPNIP {
		return true
	}
	for _, n := range ps.AllowedIPs {
		if n.Contains(dst) {
			return true
		}
	}
	return false
}

// isMulticastOrLinkLocal 判断 Windows 常经 TUN 漏出的组播/链路本地/广播探测（非真实越权单播）。
func isMulticastOrLinkLocal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if netutil.IsLimitedBroadcast(ip) {
		return true
	}
	if ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() {
		return true
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	// 224.0.0.0/4 组播；239.255.255.250 SSDP 等
	return v4[0] >= 224 && v4[0] <= 239
}

// isOtherAccountVPNIP 判断 dstIP 是否属于其他账号的 VPN IP（横向访问检测）。
func (m *Manager) isOtherAccountVPNIP(fromUserID int64, dstIP string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	owner, ok := m.vpnIndex[dstIP]
	return ok && owner != fromUserID
}

// lateralPeerAllowed 横向访问是否放行：全局开关 / peer_access / 托管路由 via 该 peer。
func (m *Manager) lateralPeerAllowed(ps *AccountSession, dstIP string) bool {
	m.mu.RLock()
	allowAll := m.allowAllPeers
	owner, ok := m.vpnIndex[dstIP]
	m.mu.RUnlock()
	if !ok || ps == nil {
		return false
	}
	if allowAll {
		return true
	}
	if _, yes := ps.PeerAccess[owner]; yes {
		return true
	}
	if _, yes := ps.ViaPeers[owner]; yes {
		return true
	}
	return false
}
