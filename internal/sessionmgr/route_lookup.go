package sessionmgr

import "net"

// route_lookup.go：入站/出站共用的会话查找辅助（按 VPN IP、viaIndex、ViaRoutes）。

// lookupOnlineByVPNIP 按虚拟 IP 查找在线会话；离线返回 nil。
func (m *Manager) lookupOnlineByVPNIP(vpnIP string) *AccountSession {
	if vpnIP == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.byIP[vpnIP]
}

// sessionIsActiveVia 判断 userID 是否为当前已加载托管路由中的 via（viaIndex 命中）。
//
// 用途：ExitLAN→对端 VPN IP 直转的信任门禁；非 via 即使广告了 local_lans 也不得绕过 peer_access。
func (m *Manager) sessionIsActiveVia(userID int64) bool {
	if userID <= 0 {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.viaIndex {
		if e.viaUserID == userID {
			return true
		}
	}
	return false
}

// lookupViaSession 在本会话 ViaRoutes 中查找 dst 的 via 在线会话。
func (m *Manager) lookupViaSession(ps *AccountSession, dst net.IP) *AccountSession {
	if ps == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range ps.ViaRoutes {
		if e.net != nil && e.net.Contains(dst) {
			if via, ok := m.sessions[e.viaUserID]; ok {
				return via
			}
		}
	}
	return nil
}
