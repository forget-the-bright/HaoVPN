package sessionmgr

import (
	"net"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// route_inbound.go：隧道入站密文处理 — HandleInbound。

// HandleInbound 处理隧道入站密文 IP 包：解密、校验源/目的 IP 后写入 TUN 或直转会话。
//
// 参数：
//   userID — 本连接鉴权后的账号；仅用于查 sessions 表。
//   conn — 产生本帧的传输连接；须与 sessions[userID].Conn 同一实例，否则静默丢弃。
//   data — Data 帧密文（nonce||ciphertext+tag）。
//   writeTUN — 将明文 IPv4 帧写入 TUN 的回调（NAT/网关流量）。
//
// 返回：解密失败时 err 非 nil；校验拒绝（伪造源、横向访问、越权目的）或陈旧 conn 时静默丢弃返回 nil。
//
// 为何必须传 conn：软重连/顶替时新旧 Conn 同 userID、同静态密钥；若只按 userID 取会话，
// 旧 readLoop 的迟到包会灌进新 Crypto 的防重放窗口（现场 ascending replay）。
// 与出站 sendToAccount / RemoveIfConn 的 Conn 身份校验对称。
//
// 副作用：累加 RxBytes；约 5s 节流刷新 session_stats；
// 对端 VPN IP（已横向放行）或托管 dest→via 均 sendToAccount，禁止依赖 TUN hairpin。
//
// via 旁路：仅当 dest 命中 viaIndex（握手 local_lans 注册）且源已通过 peer_access 时才直转 peer；
// 未注册 via 或 ExitLAN 策略拒绝时一律 writeTUN 或丢弃，禁止默认旁路工控网段。
func (m *Manager) HandleInbound(userID int64, conn PacketConn, data []byte, writeTUN func([]byte) error) error {
	m.mu.RLock()
	ps, ok := m.sessions[userID]
	m.mu.RUnlock()
	// 无会话、无 Crypto、或 conn 已不是当前会话绑定的连接 → 丢弃（不解密，避免烧新窗口）
	if !ok || ps == nil || ps.Crypto == nil || ps.Conn != conn {
		return nil
	}
	plain, err := ps.Crypto.Decrypt(data)
	if err != nil {
		return err
	}
	// 解密后再次确认会话未在 Kick/顶替窗口被替换，避免写错 TUN 路径语义。
	m.mu.RLock()
	cur, still := m.sessions[userID]
	same := still && cur == ps && cur.Conn == conn
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
		if netutil.IsTUNNoiseSource(src) || netutil.IsTUNNoiseForLog(dst) {
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
		n := ps.dstDropCount.Add(1)
		if netutil.IsTUNNoiseForLog(dst) {
			logger.Debug("丢弃越权目的 IP user_id=%d dst=%s drops=%d", userID, dstStr, n)
		} else if shouldWarnDstOverreach(ps) {
			logger.Warn("丢弃越权目的 IP user_id=%d dst=%s drops=%d（限频；客户端应滤 AllowedIPs 外目的）",
				userID, dstStr, n)
		} else {
			logger.Debug("丢弃越权目的 IP user_id=%d dst=%s drops=%d", userID, dstStr, n)
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
