package sessionmgr

import (
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
)

// Manager 维护多账号 VPN 隧道在线会话，负责注册、踢线、TUN 出站路由与入站包校验。
//
// 字段：
//   store — SQLite；连接/断开事件与 session_stats 持久化；nil 时跳过 DB 写入。
//   mu — 保护 sessions/byIP/vpnIndex 及回调字段的读写锁。
//   sessions — userID → 当前在线 AccountSession；每账号最多一条连接。
//   byIP — vpnIP → AccountSession；TUN 出站按目的 IP 快速匹配。
//   vpnIndex — vpnIP → userID；横向访问隔离（禁止 A 访问 B 的 VPN IP）。
//   onKick — 强制踢线后的回调（如禁用/改策略后通知上层）；无论是否在线都会触发。
//   onDisconnect — 断线或踢线后按 ip_mode 回收 IP 的回调。
//
// 线程安全：导出方法内部持 mu；RouteOutbound/HandleInbound 使用 RLock。
type Manager struct {
	store    *persist.Store
	mu       sync.RWMutex
	sessions map[int64]*AccountSession // userID -> session
	byIP     map[string]*AccountSession
	vpnIndex map[string]int64 // vpn_ip -> user_id（横向隔离）
	onKick   func(userID int64)
	onDisconnect func(userID int64, vpnIP, ipMode string) // IP 回收回调
}

// AccountSession 单个 VPN 账号的在线隧道会话快照。
//
// 字段：
//   UserID — 账号主键。
//   PublicKey — 客户端 WireGuard 公钥（Base64）；建立 crypto.Session 时使用。
//   VPNIP — 本次连接分配的内网 IP；fixed 模式与 DB 一致，动态模式在握手时分配。
//   AllowedIPs — 客户端可访问的目的网段（CIDR 列表）；出站路由 fallback 匹配用。
//   IPMode — IP 分配模式（fixed / dynamic_session / dynamic_lease）。
//   PolicyVer — 策略版本号；握手时下发，变更后旧连接应被踢。
//   Conn — 底层 TLS 传输连接；踢线或新连接替换时 Close。
//   Crypto — 隧道 IP 包加解密会话。
//   RemoteAddr — 客户端远端地址（host:port）。
//   RxBytes — 入站明文累计字节；atomic，约 5s 节流写入 session_stats。
//   TxBytes — 出站明文累计字节；atomic。
//   ConnectedAt — 本次连接建立时刻。
//   lastStatFlush — 上次写入 session_stats 的 UnixNano；atomic，用于流量统计节流。
type AccountSession struct {
	UserID        int64
	PublicKey     string
	VPNIP         string
	AllowedIPs    []*net.IPNet
	IPMode        string
	PolicyVer     int
	Conn          PacketConn
	Crypto        *crypto.Session
	RemoteAddr    string
	RxBytes       atomic.Int64
	TxBytes       atomic.Int64
	ConnectedAt   time.Time
	lastStatFlush atomic.Int64
}

// New 创建会话管理器实例。
//
// 参数：store — 持久化层；可为 nil（跳过 DB 写入，仅内存索引）。
// 返回：空索引表的 *Manager；须 LoadVPNIPIndex 预热 vpnIndex（若需横向隔离）。
func New(store *persist.Store) *Manager {
	return &Manager{
		store:    store,
		sessions: map[int64]*AccountSession{},
		byIP:     map[string]*AccountSession{},
		vpnIndex: map[string]int64{},
	}
}

// SetDisconnectHandler 注册 VPN 断线回调，用于按 ip_mode 回收 IP。
//
// 参数：fn — RemoveIfConn/KickUser 断线后调用；vpnIP 与 ipMode 供 ippool 回收决策。
func (m *Manager) SetDisconnectHandler(fn func(userID int64, vpnIP, ipMode string)) {
	m.onDisconnect = fn
}

// LoadVPNIPIndex 从 SQLite 预热 vpnIndex（vpn_ip → user_id 横向隔离索引）。
//
// store 为 nil 时无操作；启动后须在 RegisterVPN 前调用以启用跨账号 IP 校验。
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

// RegisterVPNIP 登记或更新账号 VPN IP 索引（开户/改 IP 后调用）。
func (m *Manager) RegisterVPNIP(vpnIP string, userID int64) {
	m.mu.Lock()
	m.vpnIndex[vpnIP] = userID
	m.mu.Unlock()
}

// UnregisterVPNIP 从横向隔离索引移除 VPN IP（删号或 IP 变更时调用）。
func (m *Manager) UnregisterVPNIP(vpnIP string) {
	m.mu.Lock()
	delete(m.vpnIndex, vpnIP)
	m.mu.Unlock()
}

// SetKickHandler 设置强制踢线回调。
//
// 参数：fn — KickUser 时无论是否在线都会调用；用于禁用/改策略等上层联动。
func (m *Manager) SetKickHandler(fn func(userID int64)) {
	m.onKick = fn
}

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

// recordDisconnect 写入 disconnected 事件并更新 session_stats（保留 reconnect_count）。
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

// OnlineCount 返回当前内存中在线 VPN 账号数。
func (m *Manager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GetSession 按 userID 查询在线会话。
//
// 返回：会话指针与是否存在；指针在会话存活期间有效，调用方不应长期持有。
func (m *Manager) GetSession(userID int64) (*AccountSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ps, ok := m.sessions[userID]
	return ps, ok
}

// ListOnline 返回所有在线账号 userID 切片（顺序不定，仅供遍历）。
func (m *Manager) ListOnline() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int64, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// ValidateVPNAccess 校验账号是否允许建立 VPN 隧道连接。
//
// 返回：无 VPN 身份或账号已禁用时 *AccessError。
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

// AccessError 表示隧道接入被拒绝（无 VPN 身份或账号已禁用）。
type AccessError struct{ Msg string }

func (e *AccessError) Error() string { return e.Msg }
