package sessionmgr

import (
	"net"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// RegisterVPN 注册或替换账号 VPN 会话（每账号最多 1 条连接）。
//
// 参数：
//   user — 已通过隧道鉴权的账号；须含 PublicKey、IPMode、PolicyVer 等。
//   allowed — 策略允许的目的 CIDR 字符串列表；由 vpnaccount 解析后传入。
//   conn — 已建立的 TLS 传输连接。
//   cryptoSess — 与服务端协商好的加密会话。
//   remoteAddr — 客户端远端地址（host:port）。
//
// 返回：allowed 解析失败时 err 非 nil。
//
// 副作用：若已有同 userID 会话则标记旧 conn 并在锁外异步 Close；更新 sessions/byIP/vpnIndex；
// 写入 connection_events（connected）与 session_stats（含 reconnect_count）；打 Info 日志。
//
// 并发：持写锁；旧连接在 goroutine 中关闭，避免在 readLoop/握手栈内同步 Close 引发死锁。
func (m *Manager) RegisterVPN(user *persist.User, allowed []string, conn PacketConn, cryptoSess *crypto.Session, remoteAddr string) error {
	var nets []*net.IPNet
	nets, err := netutil.ParseCIDRListToNets(allowed)
	if err != nil {
		return err
	}

	var oldConn PacketConn
	reconnectCount := 0
	if m.store != nil {
		// 有历史 session_stats 即视为重连（断线后 ConnectedAt 会被清空，不能依赖非空）
		if st, err := m.store.GetSessionStat(user.ID); err == nil && st != nil {
			reconnectCount = st.ReconnectCount + 1
		}
	}

	m.mu.Lock()
	if old, ok := m.sessions[user.ID]; ok {
		logger.Info("踢掉旧会话 user_id=%d（新连接）", user.ID)
		oldConn = old.Conn
		delete(m.sessions, user.ID)
		if old.VPNIP != "" {
			delete(m.byIP, old.VPNIP)
		}
	}

	ps := &AccountSession{
		UserID:      user.ID,
		PublicKey:   user.PublicKey,
		VPNIP:       user.VPNIP,
		AllowedIPs:  nets,
		IPMode:      user.IPMode,
		PolicyVer:   user.PolicyVer,
		Conn:        conn,
		Crypto:      cryptoSess,
		RemoteAddr:  remoteAddr,
		ConnectedAt: time.Now(),
	}
	m.sessions[user.ID] = ps
	if user.VPNIP != "" {
		m.byIP[user.VPNIP] = ps
		m.vpnIndex[user.VPNIP] = user.ID
	}
	m.mu.Unlock()

	if oldConn != nil {
		// 异步关闭，避免在 readLoop/握手栈内同步 Close 引发重入或死锁。
		go oldConn.Close()
	}

	now := time.Now()
	if m.store != nil {
		_ = m.store.InsertConnectionEvent(user.ID, "connected", remoteAddr, "")
		_ = m.store.UpsertSessionStat(persist.SessionStat{
			UserID:         user.ID,
			ConnectedAt:    &now,
			LastHeartbeat:  &now,
			ReconnectCount: reconnectCount,
			RemoteAddr:     remoteAddr,
		})
	}

	logger.Info("账号 %s(id=%d) 已连接 from %s vpn_ip=%s policy_ver=%d reconnect=%d",
		user.Username, user.ID, remoteAddr, user.VPNIP, user.PolicyVer, reconnectCount)
	return nil
}

// RemoveIfConn 仅当当前在线会话仍绑定指定 conn 时移除并触发断线流程。
//
// 用于 transport 读循环正常结束；避免新连接替换后误删新会话。
// 副作用：recordDisconnect、onDisconnect 回调。
func (m *Manager) RemoveIfConn(userID int64, conn PacketConn) {
	m.mu.Lock()
	ps, ok := m.sessions[userID]
	if !ok || ps.Conn != conn {
		m.mu.Unlock()
		return
	}
	delete(m.sessions, userID)
	if ps.VPNIP != "" {
		delete(m.byIP, ps.VPNIP)
	}
	vpnIP, ipMode, remoteAddr := ps.VPNIP, ps.IPMode, ps.RemoteAddr
	m.mu.Unlock()
	m.recordDisconnect(userID, remoteAddr)
	if m.onDisconnect != nil {
		m.onDisconnect(userID, vpnIP, ipMode)
	}
}
