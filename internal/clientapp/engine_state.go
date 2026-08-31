package clientapp

import (
	"context"
	"sync"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/netstack"
	"haovpn/internal/transport"
)

// State 客户端 VPN 连接状态，供 GUI/CLI 轮询展示。
type State int

const (
	StateIdle State = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

// String 返回连接状态的中文描述，供 GUI/CLI 展示。
func (s State) String() string {
	switch s {
	case StateIdle:
		return "未连接"
	case StateConnecting:
		return "连接中"
	case StateConnected:
		return "已连接"
	case StateReconnecting:
		return "重连中"
	default:
		return "未知"
	}
}

// Credentials 隧道登录凭据，由 CLI/GUI 经 SetCredentials 写入 Engine。
type Credentials struct {
	// Username Web/VPN 合一账号名，握手时发送给服务端。
	Username string
	// Password 隧道登录密码（非 Web Session）。
	Password string
	// PrivateKey 客户端隧道私钥（Base64）；GUI 记住密码模式或 zip 导入后填入。
	PrivateKey string
}

type killSwitch interface {
	Supported() error
	Enable(prefixes []string) error
	Disable() error
	Remove() error
}

type netstackKillSwitch struct{}

func (netstackKillSwitch) Supported() error        { return netstack.KillSwitchSupported() }
func (netstackKillSwitch) Enable(p []string) error { return netstack.EnableKillSwitch(p) }
func (netstackKillSwitch) Disable() error          { return netstack.DisableKillSwitch() }
func (netstackKillSwitch) Remove() error           { return netstack.RemoveKillSwitchRules() }

// Engine CLI/GUI 共用的客户端 VPN 拨号引擎。
type Engine struct {
	cfg   *config.ClientConfig
	creds Credentials
	ks    killSwitch

	mu        sync.Mutex
	state     State
	vpnIP     string
	gateway   string
	lastError string
	ksOK      bool
	// 以下字段供托盘「本机路由」只读展示（与 runtime 装路由同源，握手成功时写入）。
	managedRoutes []ManagedRouteView // peer_routes 下发的托管路由（展示 DTO）
	allowedIPs    []string              // 会话 AllowedIPs（含 NAT 工控段）
	vpnSubnet     string                // 握手 vpn_subnet；缺省时托盘可回退推导
	rt            *runtime
	reconnect *transport.ReconnectClient
	cancel    context.CancelFunc
	runCtx    context.Context

	activeMu    sync.Mutex
	activeConn  *transport.Conn
	cryptoSess  *crypto.Session
	sessionPriv string

	clearRoutesHook func()

	// failFast — GUI 登录：尚未首次鉴权成功前，拨号/握手失败即停重连并通知 WaitConnected。
	failFast bool

	// authOKOnce 是否已至少一次隧道鉴权成功；成功后关闭 failFast，断线应持续重连。
	authOKOnce bool

	// onlineRejects 「账号已在线」连续失败次数；成功握手后清零。
	onlineRejects int

	// onDataplaneFailed 鉴权成功进主窗后，TUN/路由失败时回调（GUI 回登录）；CLI 可为空。
	onDataplaneFailed func(msg string)

	// firstResult 首次鉴权结果通道；WaitConnected 等待；容量 1，只通知一次。
	// nil 表示鉴权成功（非数据面就绪）；数据面失败走 OnDataplaneFailed。
	firstResultOnce sync.Once
	firstResultCh   chan error

	// connectedAt 进入 StateConnected 的本地时间；未连接为零值（托盘「连接自」）。
	connectedAt time.Time
}

// NewEngine 创建尚未连接的客户端 VPN 引擎实例。
func NewEngine(cfg *config.ClientConfig) *Engine {
	if cfg != nil {
		// 经 netstack 门面注入 Windows 加速开关，禁止 clientapp 直接 import winnet。
		netstack.ConfigureWindows(netstack.WindowsOptions{
			UseIPHelper: cfg.Windows.UseIPHelperEnabled(),
		})
	}
	return &Engine{
		cfg:           cfg,
		state:         StateIdle,
		rt:            &runtime{cfg: cfg},
		ks:            netstackKillSwitch{},
		ksOK:          true,
		firstResultCh: make(chan error, 1),
	}
}

// SetCredentials 设置或更新隧道登录凭据。
func (e *Engine) SetCredentials(c Credentials) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.creds = c
}

// SetFailFast 设置 GUI 登录模式：首次拨号/握手失败即停止重连并唤醒 WaitConnected。
//
// CLI 长期重连应保持 false（默认）。
func (e *Engine) SetFailFast(v bool) {
	e.mu.Lock()
	e.failFast = v
	e.mu.Unlock()
}

// SetOnDataplaneFailed 注册鉴权成功后 TUN/路由失败回调（须在 Start 前调用）。
//
// GUI 应回登录窗并显示 msg；CLI 可不设。回调可能在非 UI 线程触发。
func (e *Engine) SetOnDataplaneFailed(fn func(msg string)) {
	e.mu.Lock()
	e.onDataplaneFailed = fn
	e.mu.Unlock()
}

func (e *Engine) dataplaneFailedCallback() func(string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.onDataplaneFailed
}

// State 返回当前连接状态。
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// LastError 返回最近一次对用户可见的错误文案。
func (e *Engine) LastError() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastError
}

// setLastError 写入对用户可见的错误（握手失败、杀开关等）。
func (e *Engine) setLastError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
}

// signalFirstResult 通知 WaitConnected 首次结果（仅一次；nil 表示成功）。
func (e *Engine) signalFirstResult(err error) {
	e.firstResultOnce.Do(func() {
		select {
		case e.firstResultCh <- err:
		default:
		}
	})
}

// WaitConnected 阻塞直到首次鉴权结果（成功或失败），或 ctx 取消。
//
// 成功仅表示账号握手通过，不保证 TUN/路由已就绪；数据面失败由 OnDataplaneFailed 通知。
// GUI 应在 Start 之后调用；成功后再切主界面。除 channel 外亦轮询 StateConnected 兜底。
func (e *Engine) WaitConnected(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-e.firstResultCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if e.State() == StateConnected {
				e.signalFirstResult(nil)
				return nil
			}
		}
	}
}

// KillSwitchOK 返回杀开关是否处于预期保护状态。
func (e *Engine) KillSwitchOK() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.cfg.Security.KillSwitch {
		return true
	}
	return e.ksOK
}

// VPNIP 返回握手成功后分配的虚拟 IP。
func (e *Engine) VPNIP() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vpnIP
}

// ConnectedSince 返回进入已连接状态的本地时间；未连接时为零值。
func (e *Engine) ConnectedSince() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.connectedAt
}

// Gateway 返回最近一次握手的网关 IP。
func (e *Engine) Gateway() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gateway
}

// ManagedRoutes 返回最近一次握手下发的托管路由副本（托盘只读展示）。
func (e *Engine) ManagedRoutes() []ManagedRouteView {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]ManagedRouteView{}, e.managedRoutes...)
}

// AllowedIPs 返回最近一次握手的会话分流前缀副本（含 nat.allowed_lan_cidrs 等）。
func (e *Engine) AllowedIPs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.allowedIPs...)
}

// VPNSubnet 返回握手下发的 VPN 地址池 CIDR（托盘「本机TUN」行优先使用）。
func (e *Engine) VPNSubnet() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vpnSubnet
}

func (e *Engine) setState(st State) {
	e.mu.Lock()
	e.state = st
	if st != StateConnected {
		e.connectedAt = time.Time{}
	}
	e.mu.Unlock()
}

// stopReconnectOnly 仅停止重连循环（不重复 signal）。
func (e *Engine) stopReconnectOnly() {
	e.mu.Lock()
	rc := e.reconnect
	e.mu.Unlock()
	if rc != nil {
		rc.Stop()
	}
}

// reportFirstFailure 记录失败并通知 WaitConnected（若尚未通知）。
//
// 参数 err — 须保留可 errors.Is 的哨兵（禁止仅用 fmt.Errorf("%s", msg) 剥 wrap）。
// 停重连条件：真正致命错误，或仍处登录 failFast 且尚未鉴权成功。
func (e *Engine) reportFirstFailure(err error, fatal bool) {
	if err == nil {
		return
	}
	e.setLastError(err.Error())
	e.signalFirstResult(err)
	e.mu.Lock()
	stop := fatal || (e.failFast && !e.authOKOnce)
	e.mu.Unlock()
	if stop {
		e.setState(StateIdle)
		e.stopReconnectOnly()
	}
}

// markAuthOK 标记隧道鉴权已成功：关闭 failFast，后续断线由 ReconnectClient 持续重拨。
func (e *Engine) markAuthOK() {
	e.mu.Lock()
	e.authOKOnce = true
	e.failFast = false
	e.mu.Unlock()
}

func (e *Engine) hasAuthOKOnce() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authOKOnce
}

// isReconnecting 是否处于断线重连态（用于 account_online 持续重试）。
func (e *Engine) isReconnecting() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state == StateReconnecting
}
