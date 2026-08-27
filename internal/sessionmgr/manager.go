// Package sessionmgr manages VPN account sessions and packet routing.
package sessionmgr

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/transport"
)

// Manager handles multi-account VPN sessions.
type Manager struct {
	store    *persist.Store
	mu       sync.RWMutex
	sessions map[int64]*AccountSession // userID -> session
	byIP     map[string]*AccountSession
	vpnIndex map[string]int64 // vpn_ip -> user_id（横向隔离）
	onKick   func(userID int64)
	onDisconnect func(userID int64, vpnIP, ipMode string) // IP 回收回调
}

// AccountSession 在线 VPN 账号会话。
type AccountSession struct {
	UserID        int64
	PublicKey     string
	VPNIP         string
	AllowedIPs    []*net.IPNet
	IPMode        string
	PolicyVer     int
	Conn          *transport.Conn
	Crypto        *crypto.Session
	RemoteAddr    string
	RxBytes       atomic.Int64
	TxBytes       atomic.Int64
	ConnectedAt   time.Time
	lastStatFlush atomic.Int64
}

// New creates a session manager.
func New(store *persist.Store) *Manager {
	return &Manager{
		store:    store,
		sessions: map[int64]*AccountSession{},
		byIP:     map[string]*AccountSession{},
		vpnIndex: map[string]int64{},
	}
}

// SetDisconnectHandler 断线时按 ip_mode 回收 IP。
func (m *Manager) SetDisconnectHandler(fn func(userID int64, vpnIP, ipMode string)) {
	m.onDisconnect = fn
}

// LoadVPNIPIndex 从 SQLite 加载全部账号 VPN IP 索引。
func (m *Manager) LoadVPNIPIndex() error {
	if m.store == nil {
		return nil
	}
	idx, err := m.store.GetUserVPNIPIndex()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.vpnIndex = idx
	m.mu.Unlock()
	return nil
}

// RegisterVPNIP 新建/更新账号 VPN IP 索引。
func (m *Manager) RegisterVPNIP(vpnIP string, userID int64) {
	m.mu.Lock()
	m.vpnIndex[vpnIP] = userID
	m.mu.Unlock()
}

// UnregisterVPNIP 移除 VPN IP 索引。
func (m *Manager) UnregisterVPNIP(vpnIP string) {
	m.mu.Lock()
	delete(m.vpnIndex, vpnIP)
	m.mu.Unlock()
}

// SetKickHandler sets callback for forced disconnect.
func (m *Manager) SetKickHandler(fn func(userID int64)) {
	m.onKick = fn
}

// RegisterVPN 注册或替换账号会话（每账号最多 1 连接）。
func (m *Manager) RegisterVPN(user *persist.User, allowed []string, conn *transport.Conn, cryptoSess *crypto.Session, remoteAddr string) error {
	var nets []*net.IPNet
	for _, cidr := range allowed {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			ip := net.ParseIP(cidr)
			if ip != nil {
				_, n, _ = net.ParseCIDR(cidr + "/32")
			}
		}
		if n != nil {
			nets = append(nets, n)
		}
	}
	if len(nets) == 0 {
		return fmt.Errorf("AllowedIPs 为空或全部无效，拒绝注册会话")
	}

	var oldConn *transport.Conn
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

// RemoveIfConn 仅当当前会话仍绑定该连接时移除。
func (m *Manager) RemoveIfConn(userID int64, conn *transport.Conn) {
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

// KickUser 强制断开账号隧道。
// KickUser 强制踢线：有在线会话则关闭连接；无论是否在线都触发 onKick（禁用/改策略依赖此回调）。
func (m *Manager) KickUser(userID int64) {
	m.mu.Lock()
	ps, ok := m.sessions[userID]
	var conn *transport.Conn
	var vpnIP, ipMode, remoteAddr string
	if ok {
		conn = ps.Conn
		vpnIP, ipMode, remoteAddr = ps.VPNIP, ps.IPMode, ps.RemoteAddr
		delete(m.sessions, userID)
		if ps.VPNIP != "" {
			delete(m.byIP, ps.VPNIP)
		}
	}
	m.mu.Unlock()
	if ok {
		conn.Close()
		m.recordDisconnect(userID, remoteAddr)
		if m.onDisconnect != nil {
			m.onDisconnect(userID, vpnIP, ipMode)
		}
	}
	if m.onKick != nil {
		m.onKick(userID)
	}
}

func (m *Manager) recordDisconnect(userID int64, remoteAddr string) {
	if m.store != nil {
		_ = m.store.InsertConnectionEvent(userID, "disconnected", remoteAddr, "")
		now := time.Now()
		st, _ := m.store.GetSessionStat(userID)
		rc := 0
		if st != nil {
			rc = st.ReconnectCount
		}
		_ = m.store.UpsertSessionStat(persist.SessionStat{
			UserID:         userID,
			LastHeartbeat:  &now,
			ReconnectCount: rc,
			RemoteAddr:     remoteAddr,
		})
	}
	logger.Info("账号 id=%d 已断开", userID)
}

// RouteOutbound routes a packet from TUN to the correct account.
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

// HandleInbound 处理隧道入站 IP 包：校验源/目的 IP 后写入 TUN。
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

func (m *Manager) isOtherAccountVPNIP(fromUserID int64, dstIP string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	owner, ok := m.vpnIndex[dstIP]
	return ok && owner != fromUserID
}

// OnlineCount returns number of connected accounts.
func (m *Manager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GetSession returns an account session.
func (m *Manager) GetSession(userID int64) (*AccountSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ps, ok := m.sessions[userID]
	return ps, ok
}

// ListOnline returns all online user IDs.
func (m *Manager) ListOnline() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int64, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// ValidateVPNAccess 校验账号是否允许隧道连接。
func (m *Manager) ValidateVPNAccess(user *persist.User) error {
	if user == nil || !user.HasVPN() {
		return ErrNoVPNIdentity
	}
	if !user.Enabled {
		return ErrUserDisabled
	}
	return nil
}

var (
	ErrNoVPNIdentity = &AccessError{Msg: "账号未配置 VPN 身份"}
	ErrUserDisabled  = &AccessError{Msg: "账号已禁用"}
)

// AccessError indicates tunnel access denial.
type AccessError struct{ Msg string }

func (e *AccessError) Error() string { return e.Msg }
