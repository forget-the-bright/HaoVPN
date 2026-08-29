package sessionmgr

import (
	"time"

	"haovpn/internal/logger"
)

// KickUser 强制踢线指定账号。
//
// 参数：userID — 目标账号 ID。
//
// 副作用：若在线则 Close conn、从 sessions/byIP 移除、recordDisconnect、调用 onDisconnect；
// 无论是否在线均调用 onKick（账号禁用/策略变更等场景依赖此回调）。
//
// 并发：持写锁选取会话，锁外 Close conn。
func (m *Manager) KickUser(userID int64) {
	m.mu.Lock()
	ps, ok := m.sessions[userID]
	var conn PacketConn
	var vpnIP, ipMode, remoteAddr string
	onDisc := m.onDisconnect
	onKick := m.onKick
	if ok {
		conn = ps.Conn
		vpnIP, ipMode, remoteAddr = ps.VPNIP, ps.IPMode, ps.RemoteAddr
		delete(m.sessions, userID)
		if ps.VPNIP != "" {
			delete(m.byIP, ps.VPNIP)
		}
		m.rebuildViaIndexLocked()
	}
	m.mu.Unlock()
	if ok {
		conn.Close()
		m.recordDisconnect(userID, remoteAddr)
		if onDisc != nil {
			onDisc(userID, vpnIP, ipMode)
		}
	}
	if onKick != nil {
		onKick(userID)
	}
}

// recordDisconnect 写入 disconnected 事件并更新 session_stats（保留 reconnect_count）。
func (m *Manager) recordDisconnect(userID int64, remoteAddr string) {
	if m.store != nil {
		if err := m.store.InsertConnectionEvent(userID, "disconnected", remoteAddr, ""); err != nil {
			logger.Warn("踢线写 connection_events 失败 user_id=%d: %v", userID, err)
		}
		now := time.Now()
		m.flushSessionStat(userID, nil, &now, 0, 0, remoteAddr)
	}
	logger.Info("账号 id=%d 已断开", userID)
}
