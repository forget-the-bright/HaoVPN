package sessionmgr

import (
	"net"
	"time"

	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
)

// route_policy.go：入站源/目的校验与横向访问策略（无 I/O）。
// IP 网段命中 / TUN 噪声判定委托 netutil；WARN 限频委托 safeutil.AllowEvery。

// sourceIPAllowed 入站源地址是否合法：本账号 VPN IP，或 via 出口注册网段内（LAN 回程）。
//
// ExitLAN 旁路：via 客户端经 LAN 回程时源 IP 为 PLC/工控网段而非 VPN IP，须匹配 ps.ExitLANs。
func sourceIPAllowed(ps *AccountSession, src net.IP) bool {
	if ps == nil || src == nil {
		return false
	}
	return netutil.VPNIPOrInNets(ps.VPNIP, ps.ExitLANs, src)
}

// sourceFromExitLAN 源 IP 是否落在本会话 via 广告网段（不含本账号 VPN IP 短路，仅网段）。
func sourceFromExitLAN(ps *AccountSession, src net.IP) bool {
	if ps == nil || src == nil {
		return false
	}
	return netutil.IPInAnyNet(ps.ExitLANs, src)
}

// shouldWarnSpoof 伪造源 WARN 限流：每会话至少间隔 10s 打一条。
func shouldWarnSpoof(ps *AccountSession) bool {
	if ps == nil {
		return true
	}
	return safeutil.AllowEvery(&ps.lastSpoofWarn, 10*time.Second)
}

// shouldWarnDstOverreach 越权目的 WARN 限流：每会话至少间隔 10s 打一条（对称伪造源）。
// 重连后 OS 常瞬时灌入大量公网/DNS 噪声；无限流会淹没 live 日志。
func shouldWarnDstOverreach(ps *AccountSession) bool {
	if ps == nil {
		return true
	}
	return safeutil.AllowEvery(&ps.lastDstWarn, 10*time.Second)
}

// dstAllowed 判断目的 IP 是否在该会话 VPN IP 或 AllowedIPs 网段内。
func (m *Manager) dstAllowed(ps *AccountSession, dst net.IP) bool {
	if ps == nil {
		return false
	}
	return netutil.VPNIPOrInNets(ps.VPNIP, ps.AllowedIPs, dst)
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
