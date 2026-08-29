package sessionmgr

import (
	"net"
	"strings"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

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
//   - reject_second：已有会话则：同主机 grace 顶替，或对端静默超时（半死）顶替；否则 ErrAccountAlreadyOnline；
//   - kick_previous：异步 Close 旧连接后注册新会话；
//   更新 sessions/byIP/vpnIndex/viaIndex；写 connection_events / session_stats。
//
// 并发：持写锁；旧连接在 goroutine 中关闭，避免在 readLoop/握手栈内同步 Close 引发死锁。
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
				delete(m.sessions, user.ID)
				if old.VPNIP != "" {
					delete(m.byIP, old.VPNIP)
				}
			} else {
				m.mu.Unlock()
				logger.Info("拒绝第二端登录 user_id=%d old_remote=%s new_remote=%s same_host=%v stale_peer=%v grace=%s",
					user.ID, old.RemoteAddr, remoteAddr, sameHost, stalePeer, grace)
				return ErrAccountAlreadyOnline
			}
		} else {
			// kick_previous：踢掉旧会话
			logger.Info("踢掉旧会话 user_id=%d（新连接）", user.ID)
			oldConn = old.Conn
			delete(m.sessions, user.ID)
			if old.VPNIP != "" {
				delete(m.byIP, old.VPNIP)
			}
		}
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
	m.sessions[user.ID] = ps
	if user.VPNIP != "" {
		m.byIP[user.VPNIP] = ps
		m.vpnIndex[user.VPNIP] = user.ID
	}
	m.rebuildViaIndexLocked()
	m.mu.Unlock()

	if oldConn != nil {
		// 异步关闭，避免在 readLoop/握手栈内同步 Close 引发重入或死锁。
		go oldConn.Close()
	}

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

// loadExitLANs 从临时注册表加载本账号 via 出口网段（握手前已 ReplaceClientLANRegistry）。
func (m *Manager) loadExitLANs(userID int64) []*net.IPNet {
	if m.store == nil || userID <= 0 {
		return nil
	}
	rows, err := m.store.ListClientLANRegistry(userID)
	if err != nil || len(rows) == 0 {
		return nil
	}
	var cidrs []string
	for _, r := range rows {
		cidrs = append(cidrs, r.DestCIDR)
	}
	nets, err := netutil.ParseCIDRListToNets(cidrs)
	if err != nil {
		logger.Warn("解析 exit_lans 失败 user_id=%d: %v", userID, err)
		return nil
	}
	return nets
}

// parseViaRoutes 将规格解析为 viaRouteEntry；非法 CIDR 返回错误。
func parseViaRoutes(specs []ViaRouteSpec) ([]viaRouteEntry, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]viaRouteEntry, 0, len(specs))
	for _, s := range specs {
		nets, err := netutil.ParseCIDRListToNets([]string{s.DestCIDR})
		if err != nil {
			return nil, err
		}
		if len(nets) == 0 || s.ViaUserID <= 0 {
			continue
		}
		out = append(out, viaRouteEntry{net: nets[0], viaUserID: s.ViaUserID})
	}
	return out, nil
}

// rebuildViaIndexLocked 从所有在线会话重建托管路由出站索引（调用方须持写锁）。
func (m *Manager) rebuildViaIndexLocked() {
	var idx []viaRouteEntry
	for _, ps := range m.sessions {
		idx = append(idx, ps.ViaRoutes...)
	}
	m.viaIndex = idx
}

// sameRemoteHost 比较两个 host:port 的主机部分是否相同（忽略端口；规范化 IPv4/IPv6/本机回环）。
func sameRemoteHost(a, b string) bool {
	ha := normalizeRemoteHost(a)
	hb := normalizeRemoteHost(b)
	return ha != "" && ha == hb
}

func normalizeRemoteHost(addr string) string {
	host, _ := splitHost(addr)
	host = strings.Trim(host, "[]")
	if host == "" {
		return ""
	}
	if host == "::1" || host == "0:0:0:0:0:0:0:1" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}
	return strings.ToLower(host)
}

func splitHost(addr string) (host, port string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, ""
	}
	return host, port
}

// reconnectStaleAfter 半死会话判定阈值：对端静默超过此时间则允许密码重连顶替。
// 取 grace 与 20s 中较小者（至少 8s），以便客户端持续重试窗口内能顶替 ZT 黑洞会话。
func reconnectStaleAfter(grace time.Duration) time.Duration {
	const minStale = 8 * time.Second
	const maxStale = 20 * time.Second
	if grace <= 0 {
		return maxStale
	}
	d := grace
	if d > maxStale {
		d = maxStale
	}
	if d < minStale {
		d = minStale
	}
	return d
}

// peerActivityStale 旧连接对端是否已静默超过阈值（实现 PeerActivityConn 时才可判定）。
func peerActivityStale(conn PacketConn, after time.Duration) bool {
	if conn == nil || after <= 0 {
		return false
	}
	pa, ok := conn.(PeerActivityConn)
	if !ok {
		return false
	}
	t := pa.LastPeerActivity()
	if t.IsZero() {
		return false
	}
	return time.Since(t) >= after
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
