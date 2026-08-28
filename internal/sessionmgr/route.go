package sessionmgr

import (
	"net"
	"sort"
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// RouteOutbound 将 TUN 出站 IPv4 包路由到匹配的在线账号并加密发送。
//
// 参数：packet — 原始 IPv4 帧；长度须 ≥ 20 字节以便读取目的地址。
//
// 返回：true 表示已找到匹配账号并成功发送；false 表示包过短、无匹配账号或加密/发送失败。
//
// 副作用：匹配账号的 TxBytes 累加；经 transport.Conn 发送密文帧。
//
// 并发：持 RLock 遍历选路（userID 升序保证确定性）；sendToAccount 无额外锁。
func (m *Manager) RouteOutbound(packet []byte) bool {
	if len(packet) < 20 {
		return false
	}
	dst := net.IP(packet[16:20])
	dstStr := dst.String()

	m.mu.RLock()
	ids := make([]int64, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var target *AccountSession
	for _, id := range ids {
		ps := m.sessions[id]
		if ps.VPNIP == dstStr {
			target = ps
			break
		}
	}
	if target == nil {
		for _, id := range ids {
			ps := m.sessions[id]
			for _, n := range ps.AllowedIPs {
				if n.Contains(dst) {
					target = ps
					break
				}
			}
			if target != nil {
				break
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
func (m *Manager) sendToAccount(ps *AccountSession, packet []byte) bool {
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

// HandleInbound 处理隧道入站密文 IP 包：解密、校验源/目的 IP 后写入 TUN。
//
// 参数：writeTUN — 将明文 IPv4 帧写入 TUN 的回调。
// 返回：解密失败时 err 非 nil；校验拒绝（伪造源、横向访问、越权目的）时静默丢弃返回 nil。
// 副作用：累加 RxBytes；约 5s 节流刷新 session_stats。
func (m *Manager) HandleInbound(userID int64, data []byte, writeTUN func([]byte) error) error {
	m.mu.RLock()
	ps, ok := m.sessions[userID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	plain, err := ps.Crypto.Decrypt(data)
	if err != nil {
		return err
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

	if ps.VPNIP != "" && srcStr != ps.VPNIP {
			if srcStr == "0.0.0.0" {
				logger.Debug("丢弃伪造源 IP 包 user_id=%d src=%s 期望=%s", userID, srcStr, ps.VPNIP)
			} else {
				logger.Warn("丢弃伪造源 IP 包 user_id=%d src=%s 期望=%s", userID, srcStr, ps.VPNIP)
			}
			return nil
		}
		if m.isOtherAccountVPNIP(userID, dstStr) {
			logger.Warn("阻断横向访问 user_id=%d dst=%s", userID, dstStr)
			return nil
		}
		if !m.dstAllowed(ps, dst) {
			if isMulticastOrLinkLocal(dst) {
				logger.Debug("丢弃越权目的 IP user_id=%d dst=%s", userID, dstStr)
			} else {
				logger.Warn("丢弃越权目的 IP user_id=%d dst=%s", userID, dstStr)
			}
			return nil
		}
	ps.RxBytes.Add(int64(len(plain)))
	now := time.Now()
	last := ps.lastStatFlush.Load()
	if now.UnixNano()-last >= int64(5*time.Second) {
		if ps.lastStatFlush.CompareAndSwap(last, now.UnixNano()) && m.store != nil {
			rc := 0
			if st, err := m.store.GetSessionStat(userID); err == nil {
				rc = st.ReconnectCount
			}
			_ = m.store.UpsertSessionStat(persist.SessionStat{
				UserID:         userID,
				ConnectedAt:    &ps.ConnectedAt,
				LastHeartbeat:  &now,
				RxBytes:        ps.RxBytes.Load(),
				TxBytes:        ps.TxBytes.Load(),
				ReconnectCount: rc,
				RemoteAddr:     ps.RemoteAddr,
			})
		}
	}
	return writeTUN(plain)
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

// isMulticastOrLinkLocal 判断 Windows 常经 TUN 漏出的组播/链路本地探测（非真实越权单播）。
func isMulticastOrLinkLocal(ip net.IP) bool {
	if ip == nil {
		return false
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
