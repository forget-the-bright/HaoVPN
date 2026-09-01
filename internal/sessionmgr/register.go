package sessionmgr

import (
	"net"
	"time"

	"haovpn/internal/auth"
	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// register.go：RegisterVPN 主路径 + 旧 Conn 排空 + RemoveIfConn。
// grace/顶替判定见 register_grace.go；ExitLAN / viaIndex / lan_registry 见 register_lan.go。

// RegisterVPN 注册账号 VPN 会话（每账号最多 1 条连接）；peer 可为空。
//
// 参数：
//   user — 已通过隧道鉴权的账号；须含 PublicKey、IPMode、PolicyVer 等。
//   allowed — 策略允许的目的 CIDR 字符串列表；由 vpnaccount 解析后传入。
//   conn — 已建立的 TLS 传输连接。
//   cryptoSess — 与服务端协商好的加密会话。
//   remoteAddr — 客户端远端地址（host:port）。
//   peer — 互访白名单与托管路由（可选）。
//
// 返回：allowed 解析失败，或 session_policy=reject_second 且账号已在线（且不满足 grace）时 err 非 nil。
//
// 副作用：
//   - reject_second：已有会话则：同主机 grace 顶替，或对端静默超时（半死）顶替；否则 auth.ErrAccountAlreadyOnline；
//   - kick_previous / grace 顶替：先摘旧会话，清 Data 回调，同步 Close 并等待 Done（旧 readLoop 退出），
//     再挂新 Crypto（避免同钥迟到包占防重放窗口；local_lans/ICS 长配网软重连现场）；
//   更新 sessions/byIP/vpnIndex/viaIndex；写 connection_events / session_stats。
//
// 并发：持写锁摘旧/挂新；旧连接在解锁后由本函数同步 Close+排空（调用方须为新连接握手栈，勿在旧 readLoop 内调 RegisterVPN）。
func (m *Manager) RegisterVPN(user *persist.User, allowed []string, conn PacketConn, cryptoSess *crypto.Session, remoteAddr string, peer PeerReg) error {
	var nets []*net.IPNet
	nets, err := netutil.ParseCIDRListToNets(allowed)
	if err != nil {
		return err
	}
	viaRoutes, err := parseViaRoutes(peer.ViaRoutes)
	if err != nil {
		return err
	}
	peerAccess := make(map[int64]struct{}, len(peer.PeerAccessIDs))
	for _, id := range peer.PeerAccessIDs {
		peerAccess[id] = struct{}{}
	}
	viaPeers := make(map[int64]struct{}, len(peer.ViaUserIDs))
	for _, id := range peer.ViaUserIDs {
		viaPeers[id] = struct{}{}
	}

	var oldConn PacketConn
	inheritRx, inheritTx := int64(0), int64(0)
	reconnectCount := 0
	if m.store != nil {
		// 有历史 session_stats 即视为重连（断线后 ConnectedAt 会被清空，不能依赖非空）
		if st, err := m.store.GetSessionStat(user.ID); err == nil && st != nil {
			reconnectCount = st.ReconnectCount + 1
		}
	}

	exitLANs := m.loadExitLANs(user.ID)

	m.mu.Lock()
	policy := m.sessionPolicy
	if policy == "" {
		policy = config.SessionPolicyRejectSecond
	}
	grace := m.reconnectGrace
	if old, ok := m.sessions[user.ID]; ok {
		sameHost := sameRemoteHost(old.RemoteAddr, remoteAddr)
		stalePeer := peerActivityStale(old.Conn, reconnectStaleAfter(grace))
		if policy == config.SessionPolicyRejectSecond {
			// 顶替条件（须 grace>0）：
			// 1) 同公网主机（抖动换端口）；或
			// 2) 旧连接对端已静默超过阈值（ZT/黑洞半死，IP 可能因 frp/NAT 看起来不同）
			if grace > 0 && (sameHost || stalePeer) {
				reason := "same_ip"
				if !sameHost {
					reason = "stale_peer"
				}
				logger.Info("grace 顶替旧会话 user_id=%d reason=%s old_remote=%s new_remote=%s inherit rx/tx",
					user.ID, reason, old.RemoteAddr, remoteAddr)
				inheritRx = old.RxBytes.Load()
				inheritTx = old.TxBytes.Load()
				oldConn = old.Conn
				// 先摘会话：旧 Conn 再入站时 sessions miss → 丢弃，避免同钥旧 counter 灌进新 Crypto
				old.Crypto = nil
				delete(m.sessions, user.ID)
				if old.VPNIP != "" {
					delete(m.byIP, old.VPNIP)
				}
			} else {
				m.mu.Unlock()
				logger.Info("拒绝第二端登录 user_id=%d old_remote=%s new_remote=%s same_host=%v stale_peer=%v grace=%s",
					user.ID, old.RemoteAddr, remoteAddr, sameHost, stalePeer, grace)
				return auth.ErrAccountAlreadyOnline
			}
		} else {
			// kick_previous：踢掉旧会话
			logger.Info("踢掉旧会话 user_id=%d（新连接）", user.ID)
			oldConn = old.Conn
			old.Crypto = nil
			delete(m.sessions, user.ID)
			if old.VPNIP != "" {
				delete(m.byIP, old.VPNIP)
			}
		}
	}
	m.mu.Unlock()

	// 在挂新 Crypto 之前：清回调 → Close → 等 Done。
	// 为何：仅 Close 不保证 readLoop 已退出；旧 onData 仍可能在新会话挂上后调用 HandleInbound。
	// HandleInbound 已按 Conn 身份校验作第二道防线；此处排空缩小竞态窗口。
	if oldConn != nil {
		drainOldConn(oldConn, user.ID)
	}

	ps := &AccountSession{
		UserID:      user.ID,
		PublicKey:   user.PublicKey,
		VPNIP:       user.VPNIP,
		AllowedIPs:  nets,
		ViaRoutes:   viaRoutes,
		PeerAccess:  peerAccess,
		ViaPeers:    viaPeers,
		ExitLANs:    exitLANs,
		IPMode:      user.IPMode,
		PolicyVer:   user.PolicyVer,
		Conn:        conn,
		Crypto:      cryptoSess,
		RemoteAddr:  remoteAddr,
		ConnectedAt: time.Now(),
	}
	ps.RxBytes.Store(inheritRx)
	ps.TxBytes.Store(inheritTx)

	m.mu.Lock()
	m.sessions[user.ID] = ps
	if user.VPNIP != "" {
		m.byIP[user.VPNIP] = ps
		m.vpnIndex[user.VPNIP] = user.ID
	}
	m.rebuildViaIndexLocked()
	m.mu.Unlock()

	now := time.Now()
	if m.store != nil {
		if err := m.store.InsertConnectionEvent(user.ID, "connected", remoteAddr, ""); err != nil {
			logger.Warn("写 connection_events 失败 user_id=%d: %v", user.ID, err)
		}
		if err := m.store.UpsertSessionStat(persist.SessionStat{
			UserID:         user.ID,
			ConnectedAt:    &now,
			LastHeartbeat:  &now,
			RxBytes:        inheritRx,
			TxBytes:        inheritTx,
			ReconnectCount: reconnectCount,
			RemoteAddr:     remoteAddr,
		}); err != nil {
			logger.Warn("写 session_stats 失败 user_id=%d: %v", user.ID, err)
		}
	}

	logger.Info("账号 %s(id=%d) 已连接 from %s vpn_ip=%s policy_ver=%d reconnect=%d inherit_rx=%d inherit_tx=%d peers=%d vias=%d exit_lans=%d",
		user.Username, user.ID, remoteAddr, user.VPNIP, user.PolicyVer, reconnectCount, inheritRx, inheritTx,
		len(peerAccess), len(viaPeers), len(exitLANs))
	return nil
}

// oldConnDrainTimeout 等待旧 Conn readLoop 退出的上限。
// 过短仍可能残留迟到包（靠 HandleInbound Conn 校验兜底）；过长拖慢握手。
const oldConnDrainTimeout = 2 * time.Second

// drainOldConn 清除 Data 回调、Close，并在实现 DrainableConn 时等待 Done。
//
// 参数：old — 被顶替的旧连接；userID — 仅用于超时日志。
// 为何先 SetOnData(nil)：即使 readLoop 仍在读，也不再进入业务解密路径。
func drainOldConn(old PacketConn, userID int64) {
	if old == nil {
		return
	}
	if dc, ok := old.(DataCallbackConn); ok {
		dc.SetOnData(nil)
	}
	_ = old.Close()
	dr, ok := old.(DrainableConn)
	if !ok {
		return
	}
	done := dr.Done()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(oldConnDrainTimeout):
		logger.Warn("old_conn_drain_timeout user_id=%d wait=%s", userID, oldConnDrainTimeout)
	}
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
	m.rebuildViaIndexLocked()
	vpnIP, ipMode, remoteAddr := ps.VPNIP, ps.IPMode, ps.RemoteAddr
	onDisc := m.onDisconnect
	m.mu.Unlock()
	m.recordDisconnect(userID, remoteAddr)
	if onDisc != nil {
		onDisc(userID, vpnIP, ipMode)
	}
}
