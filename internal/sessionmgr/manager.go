package sessionmgr

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/flowmon"
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
//   viaIndex — 托管路由 dest→viaUserID（会话注册时合并）；出站按 via 转发，禁止用 AllowedIPs（NAT）错送。
//   allowAllPeers — security.allow_all_vpn_peers；true 时任意账号可互访对方 VPN IP。
//   sessionPolicy — reject_second（默认）或 kick_previous；见 config.SessionPolicy*。
//   reconnectGrace — 同公网 IP 短窗内顶替旧会话并续算流量；0=关闭。
//   onKick — 强制踢线后的回调（如禁用/改策略后通知上层）；无论是否在线都会触发。
//   onDisconnect — 断线或踢线后按 ip_mode 回收 IP 的回调。
//   Flows — 可选 L4 流表；nil 时跳过 Observe。
//
// 账号已在线哨兵：auth.ErrAccountAlreadyOnline（本包不再 re-export 别名）。
//
// 线程安全：导出方法内部持 mu；RouteOutbound/HandleInbound 使用 RLock。
type Manager struct {
	store          *persist.Store
	mu             sync.RWMutex
	sessions       map[int64]*AccountSession // userID -> session
	byIP           map[string]*AccountSession
	vpnIndex       map[string]int64 // vpn_ip -> user_id（横向隔离）
	viaIndex       []viaRouteEntry  // 托管路由出站索引
	allowAllPeers  bool
	sessionPolicy  string
	reconnectGrace time.Duration
	onKick         func(userID int64)
	onDisconnect   func(userID int64, vpnIP, ipMode string) // IP 回收回调
	Flows          *flowmon.Tracker                         // L4 流表；可为 nil
}

// viaRouteEntry 托管路由出站一项：命中 dest 网段则转发到 viaUserID 会话。
type viaRouteEntry struct {
	net       *net.IPNet
	viaUserID int64
}

// AccountSession 单个 VPN 账号的在线隧道会话快照。
//
// 字段：
//   UserID — 账号主键。
//   PublicKey — 客户端 WireGuard 公钥（Base64）；建立 crypto.Session 时使用。
//   VPNIP — 本次连接分配的内网 IP；fixed 模式与 DB 一致，动态模式在握手时分配。
//   AllowedIPs — 客户端可访问的目的网段（CIDR 列表）；入站 dstAllowed 用；**不出站**按此匹配（避免 NAT 网段错送回客户端）。
//   ViaRoutes — 本账号安装的托管路由（dest→via）；入站可直转 via，并重建 Manager.viaIndex。
//   PeerAccess — 白名单 peer user_id；可直连对方 VPN IP。
//   ViaPeers — 托管路由 via 账号集合；亦可直连其 VPN IP（下一跳可达）。
//   IPMode — IP 分配模式（fixed / dynamic_session / dynamic_lease）。
//   PolicyVer — 策略版本号；握手时下发，变更后旧连接应被踢。
//   Conn — 底层 TLS 传输连接；踢线或新连接替换时 Close。
//   Crypto — 隧道 IP 包加解密会话。
//   RemoteAddr — 客户端远端地址（host:port）。
//   RxBytes — 入站明文累计字节；atomic，约 5s 节流写入 session_stats。
//   TxBytes — 出站明文累计字节；atomic。
//   ConnectedAt — 本次连接建立时刻。
//   lastStatFlush — 上次写入 session_stats 的 UnixNano；atomic，用于流量统计节流。
//   ExitLANs — via 出口广告网段（来自 client_lan_registry）；允许这些源 IP 入站回程（否则 SNAT 回程会被当伪造源丢掉）。
//   lastSpoofWarn — 伪造源 WARN 限流时间戳（UnixNano）。
//   lastDstWarn — 越权目的 WARN 限流时间戳（UnixNano）。
//   dstDropCount — 越权目的累计丢弃次数（限频 WARN 时打出）。
//   lanRegistrySyncCount / lastLANRegistrySync — 本会话 post-auth lan_registry 次数与时间，防刷库。
type AccountSession struct {
	UserID        int64
	PublicKey     string
	VPNIP         string
	AllowedIPs    []*net.IPNet
	ViaRoutes     []viaRouteEntry
	PeerAccess    map[int64]struct{}
	ViaPeers      map[int64]struct{}
	ExitLANs      []*net.IPNet
	IPMode        string
	PolicyVer     int
	Conn          PacketConn
	Crypto        *crypto.Session
	RemoteAddr    string
	RxBytes       atomic.Int64
	TxBytes       atomic.Int64
	ConnectedAt   time.Time
	lastStatFlush atomic.Int64
	lastSpoofWarn atomic.Int64
	lastDstWarn   atomic.Int64
	dstDropCount  atomic.Uint64
	// lanRegistrySyncCount 本连接已接受的 lan_registry 次数（含本次数）。
	lanRegistrySyncCount int
	// lastLANRegistrySync 上次接受 lan_registry 的时间。
	lastLANRegistrySync time.Time
}

// PeerReg 注册会话时附带的 peer/托管路由策略（由 vpnaccount.ResolveClientPolicy 填充）。
type PeerReg struct {
	ViaRoutes     []ViaRouteSpec // dest CIDR + via user
	PeerAccessIDs []int64
	ViaUserIDs    []int64
}

// ViaRouteSpec 托管路由注册规格（字符串 CIDR + via 账号）。
type ViaRouteSpec struct {
	DestCIDR  string
	ViaUserID int64
}

// New 创建会话管理器实例（默认 session_policy=reject_second）。
//
// 参数：store — 持久化层；可为 nil（跳过 DB 写入，仅内存索引）。
// 返回：空索引表的 *Manager；须 LoadVPNIPIndex 预热 vpnIndex（若需横向隔离）。
func New(store *persist.Store) *Manager {
	return &Manager{
		store:         store,
		sessions:      map[int64]*AccountSession{},
		byIP:          map[string]*AccountSession{},
		vpnIndex:      map[string]int64{},
		sessionPolicy: config.SessionPolicyRejectSecond,
		Flows:         flowmon.New(flowmon.Options{}),
	}
}

// SetAllowAllVPNPeers 设置是否允许全部 VPN 账号互访对方虚拟 IP。
func (m *Manager) SetAllowAllVPNPeers(allow bool) {
	m.mu.Lock()
	m.allowAllPeers = allow
	m.mu.Unlock()
}

// SetReconnectGrace 设置同公网 IP 短窗顶替并续算流量的宽限；0 表示关闭。
func (m *Manager) SetReconnectGrace(d time.Duration) {
	m.mu.Lock()
	if d < 0 {
		d = 0
	}
	m.reconnectGrace = d
	m.mu.Unlock()
}

// SetSessionPolicy 设置同账号第二端策略（reject_second / kick_previous）。
//
// 空字符串视为 reject_second；非法值忽略并保持原策略。
func (m *Manager) SetSessionPolicy(policy string) {
	switch policy {
	case config.SessionPolicyRejectSecond, config.SessionPolicyKickPrevious:
		m.mu.Lock()
		m.sessionPolicy = policy
		m.mu.Unlock()
	case "":
		m.mu.Lock()
		m.sessionPolicy = config.SessionPolicyRejectSecond
		m.mu.Unlock()
	}
}

// SetDisconnectHandler 注册 VPN 断线回调，用于按 ip_mode 回收 IP。
//
// 参数：fn — RemoveIfConn/KickUser 断线后调用；vpnIP 与 ipMode 供 ippool 回收决策。
// 并发：持写锁赋值，与 KickUser/RemoveIfConn 读回调一致，避免数据竞争。
func (m *Manager) SetDisconnectHandler(fn func(userID int64, vpnIP, ipMode string)) {
	m.mu.Lock()
	m.onDisconnect = fn
	m.mu.Unlock()
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
// 并发：持写锁赋值，与 KickUser 读回调一致。
func (m *Manager) SetKickHandler(fn func(userID int64)) {
	m.mu.Lock()
	m.onKick = fn
	m.mu.Unlock()
}

