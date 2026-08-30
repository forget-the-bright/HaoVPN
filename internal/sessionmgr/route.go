package sessionmgr

import (
	"net"

	"haovpn/internal/logger"
)

// route.go：TUN 出站选路 — RouteOutbound 与 sendToAccount。
// 入站、查找与策略校验拆到 route_inbound / route_lookup / route_policy。

// RouteOutbound 将 TUN 出站 IPv4 包路由到匹配的在线账号并加密发送。
//
// 匹配顺序：
//  1. byIP[dst] — O(1) 命中某账号 VPN IP；
//  2. viaIndex 命中托管路由 dest → via 账号会话；
//  3. **不再**用会话 AllowedIPs（NAT 工控网段）匹配，避免把应 NAT 的流量错送回客户端。
//
// 参数：packet — 原始 IPv4 帧；长度须 ≥ 20 字节以便读取目的地址。
//
// 返回：true 表示已找到匹配账号并成功发送；false 表示包过短、无匹配账号或加密/发送失败。
//
// 副作用：匹配账号的 TxBytes 累加；经 transport.Conn 发送密文帧。
//
// 并发：持 RLock 选路；sendToAccount 无额外锁。
func (m *Manager) RouteOutbound(packet []byte) bool {
	if len(packet) < 20 {
		return false
	}
	dst := net.IP(packet[16:20])
	dstStr := dst.String()

	m.mu.RLock()
	var target *AccountSession
	if ps := m.byIP[dstStr]; ps != nil {
		target = ps
	} else {
		for _, e := range m.viaIndex {
			if e.net != nil && e.net.Contains(dst) {
				if ps, ok := m.sessions[e.viaUserID]; ok {
					target = ps
					break
				}
			}
		}
	}
	m.mu.RUnlock()

	if target == nil {
		return false
	}
	return m.sendToAccount(target, packet)
}

// sendToAccount 加密并向指定在线账号发送出站 IP 包。
//
// 返回：加密或 Send 失败时 false；成功时累加 TxBytes。
// 并发：发送前再确认 sessions 仍绑定同一 Conn，缩小 Kick 窗口下的错连风险。
func (m *Manager) sendToAccount(ps *AccountSession, packet []byte) bool {
	if ps == nil || ps.Conn == nil || ps.Crypto == nil {
		return false
	}
	m.mu.RLock()
	cur, ok := m.sessions[ps.UserID]
	same := ok && cur == ps && cur.Conn == ps.Conn
	m.mu.RUnlock()
	if !same {
		return false
	}
	enc, err := ps.Crypto.Encrypt(packet)
	if err != nil {
		logger.Warn("加密失败 user_id=%d: %v", ps.UserID, err)
		return false
	}
	if err := ps.Conn.Send(enc); err != nil {
		logger.Warn("发送失败 user_id=%d: %v", ps.UserID, err)
		return false
	}
	ps.TxBytes.Add(int64(len(packet)))
	return true
}
