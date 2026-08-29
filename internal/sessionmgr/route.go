package sessionmgr

import (
	"net"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

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

// HandleInbound 处理隧道入站密文 IP 包：解密、校验源/目的 IP 后写入 TUN 或直转会话。
//
// 参数：writeTUN — 将明文 IPv4 帧写入 TUN 的回调（NAT/网关流量）。
// 返回：解密失败时 err 非 nil；校验拒绝（伪造源、横向访问、越权目的）时静默丢弃返回 nil。
// 副作用：累加 RxBytes；约 5s 节流刷新 session_stats；
// 对端 VPN IP（已横向放行）或托管 dest→via 均 sendToAccount，禁止依赖 TUN hairpin。
func (m *Manager) HandleInbound(userID int64, data []byte, writeTUN func([]byte) error) error {
	m.mu.RLock()
	ps, ok := m.sessions[userID]
	m.mu.RUnlock()
	if !ok || ps == nil || ps.Crypto == nil {
		return nil
	}
	plain, err := ps.Crypto.Decrypt(data)
	if err != nil {
		return err
	}
	// 解密后再次确认会话未在 Kick 窗口被替换，避免写错 TUN 路径语义。
	m.mu.RLock()
	cur, still := m.sessions[userID]
	same := still && cur == ps
	m.mu.RUnlock()
	if !same {
		return nil
	}
	if len(plain) < 20 {
		logger.Warn("丢弃过短入站包 user_id=%d len=%d", userID, len(plain))
		return nil
	}
	if plain[0]>>4 != 4 {
		// Windows 常向 TUN 灌 IPv6 探测；本产品仅转发 IPv4，降为 DEBUG 避免刷屏。
		logger.Debug("丢弃非 IPv4 入站包 user_id=%d ver=%d", userID, plain[0]>>4)
		return nil
	}
	src := net.IP(plain[12:16])
	dst := net.IP(plain[16:20])
	srcStr, dstStr := src.String(), dst.String()

	if !sourceIPAllowed(ps, src) {
		// 广播/链路噪声与常见误注入：降级 DEBUG；其余伪造源限流 WARN（via ICS 刷屏）
		if isNoiseSourceIP(src) || isMulticastOrLinkLocal(dst) {
			logger.Debug("丢弃伪造源 IP 包 user_id=%d src=%s 期望=%s", userID, srcStr, ps.VPNIP)
		} else if shouldWarnSpoof(ps) {
			logger.Warn("丢弃伪造源 IP 包 user_id=%d src=%s 期望=%s（非 VPN IP 且不在 exit_lans；via 须上报 local_lans）",
				userID, srcStr, ps.VPNIP)
		} else {
			logger.Debug("丢弃伪造源 IP 包 user_id=%d src=%s 期望=%s", userID, srcStr, ps.VPNIP)
		}
		return nil
	}

	// via 出口回程：源在 ExitLANs、目的为其他账号 VPN IP → 直转（不要求 peer_access / dstAllowed）。
	// 信任边界：仅当本会话是「已应用托管路由」中的 via（viaIndex 命中）才允许该旁路。
	// 非 via 即使有 ExitLANs，也落入下方 lateralPeerAllowed，不得绕过 peer_access。
	if sourceFromExitLAN(ps, src) && m.sessionIsActiveVia(userID) {
		if peer := m.lookupOnlineByVPNIP(dstStr); peer != nil && peer.UserID != userID {
			m.noteInboundRx(ps, userID, len(plain))
			_ = m.sendToAccount(peer, plain)
			return nil
		}
	}

	lateralPeer := false
	if m.isOtherAccountVPNIP(userID, dstStr) {
		if !m.lateralPeerAllowed(ps, dstStr) {
			logger.Warn("阻断横向访问 user_id=%d dst=%s", userID, dstStr)
			return nil
		}
		lateralPeer = true
	}
	if !m.dstAllowed(ps, dst) {
		if isMulticastOrLinkLocal(dst) || netutil.IsLimitedBroadcast(dst) {
			logger.Debug("丢弃越权目的 IP user_id=%d dst=%s", userID, dstStr)
		} else {
			logger.Warn("丢弃越权目的 IP user_id=%d dst=%s", userID, dstStr)
		}
		return nil
	}
	m.noteInboundRx(ps, userID, len(plain))

	// 横向互访：直转对端会话（hub-and-spoke，禁止 writeTUN 指望 hairpin）
	if lateralPeer {
		if peer := m.lookupOnlineByVPNIP(dstStr); peer != nil {
			_ = m.sendToAccount(peer, plain)
			return nil
		}
		logger.Debug("横向目标离线 user_id=%d dst=%s", userID, dstStr)
		return nil
	}

	// 托管路由：命中 dest 则直转 via 会话（服务端内核通常无该 LAN 路由）
	if via := m.lookupViaSession(ps, dst); via != nil {
		_ = m.sendToAccount(via, plain)
		return nil
	}
	return writeTUN(plain)
}

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

// noteInboundRx 累加入站字节并节流刷新 session_stats。
func (m *Manager) noteInboundRx(ps *AccountSession, userID int64, n int) {
	if ps == nil || n <= 0 {
		return
	}
	ps.RxBytes.Add(int64(n))
	now := time.Now()
	last := ps.lastStatFlush.Load()
	if now.UnixNano()-last < int64(5*time.Second) {
		return
	}
	if !ps.lastStatFlush.CompareAndSwap(last, now.UnixNano()) || m.store == nil {
		return
	}
	m.flushSessionStat(userID, &ps.ConnectedAt, &now, ps.RxBytes.Load(), ps.TxBytes.Load(), ps.RemoteAddr)
}

// flushSessionStat 写入/更新 session_stats，保留既有 reconnect_count。
//
// route 节流刷新与 kick 断线共用，避免两处字段漂移。
func (m *Manager) flushSessionStat(userID int64, connectedAt, lastHB *time.Time, rx, tx int64, remoteAddr string) {
	if m.store == nil {
		return
	}
	rc := 0
	if st, err := m.store.GetSessionStat(userID); err == nil && st != nil {
		rc = st.ReconnectCount
	}
	if err := m.store.UpsertSessionStat(persist.SessionStat{
		UserID:         userID,
		ConnectedAt:    connectedAt,
		LastHeartbeat:  lastHB,
		RxBytes:        rx,
		TxBytes:        tx,
		ReconnectCount: rc,
		RemoteAddr:     remoteAddr,
	}); err != nil {
		logger.Warn("更新 session_stats 失败 user_id=%d: %v", userID, err)
	}
}

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
