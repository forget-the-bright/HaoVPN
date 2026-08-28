package clientapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/netutil"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
)

// State 客户端 VPN 连接状态，供 GUI/CLI 轮询展示。
//
// 取值由 Engine 在 Start、onConnect、onClose 与 Stop 中更新；State() 读取时持 mu 锁。
type State int

const (
	// StateIdle 未连接或已 Stop；vpnIP 为空，重连循环未运行或已取消。
	StateIdle State = iota
	// StateConnecting 正在建立 TLS 或等待握手/策略应用。
	StateConnecting
	// StateConnected 握手成功且策略已应用；TUN 与路由就绪，数据面可收发。
	StateConnected
	// StateReconnecting 隧道已断开，transport 正在自动重连；可能已启用杀开关并清除路由。
	StateReconnecting
)

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
//
// 字段：
//   Username — VPN 账号名；握手前 TrimSpace，空则拒绝连接。
//   Password — 明文密码；用于 tunnel 鉴权，握手成功前不得为空。
//   PrivateKey — 可选 WireGuard 风格私钥；握手应答未下发时作回退；正常密码登录以服务端下发为准。
type Credentials struct {
	Username   string
	Password   string
	PrivateKey string // 旧导出兼容；密码登录时由握手下发覆盖
}

// killSwitch 杀开关操作（可注入假实现做顺序单测）。
type killSwitch interface {
	Supported() error
	Enable(prefixes []string) error
	Disable() error
	Remove() error
}

// netstackKillSwitch 默认走 WFP/平台实现。
type netstackKillSwitch struct{}

func (netstackKillSwitch) Supported() error              { return netstack.KillSwitchSupported() }
func (netstackKillSwitch) Enable(p []string) error       { return netstack.EnableKillSwitch(p) }
func (netstackKillSwitch) Disable() error                { return netstack.DisableKillSwitch() }
func (netstackKillSwitch) Remove() error                 { return netstack.RemoveKillSwitchRules() }

// Engine CLI/GUI 共用的客户端 VPN 拨号引擎：TLS 重连、隧道握手、TUN/路由与杀开关编排。
//
// 字段：
//   cfg — 只读客户端配置指针；生命周期由调用方保证，Engine 不拷贝配置。
//   creds — 当前登录凭据；SetCredentials 写入，onConnect 读取时需持 mu。
//   ks — 杀开关实现；默认 netstackKillSwitch，单测可注入假实现。
//   mu — 保护 state、vpnIP、gateway、lastError、ksOK、creds、cancel/reconnect 等可见状态。
//   state — 连接状态枚举，见 State。
//   vpnIP — 握手成功后分配的虚拟 IP；未连接或 Stop 后为空。
//   gateway — 服务端下发的网关 IP，供 runtime 添加路由时使用。
//   lastError — 最近一次对用户可见的错误文案（如杀开关失败）；空表示无。
//   ksOK — 杀开关是否处于预期保护状态；未开启杀开关配置时恒为 true。
//   rt — TUN 设备与路由运行时；与数据面读写同包内协作。
//   reconnect — transport 重连客户端；Start 创建，Stop 停止。
//   cancel / runCtx — Start 创建的根上下文；Stop 调用 cancel 以结束 tunReadLoop。
//   activeMu — 保护 activeConn、cryptoSess、sessionPriv；锁序须先于 mu（见 Stop/onClose）。
//   activeConn — 当前活跃 TLS 隧道连接；断线或 Stop 时置 nil。
//   cryptoSess — 与 activeConn 配对的加解密会话；握手下发密钥后建立。
//   sessionPriv — 当前会话客户端私钥字符串；供调试或后续扩展，Stop 时清空。
//   clearRoutesHook — 仅单测：clearRoutes 前回调，用于断言杀开关与清路由顺序。
//
// 线程安全：State/LastError/KillSwitchOK/VPNIP 等查询方法持 mu；Start/Stop 应由单 goroutine 串行调用。
type Engine struct {
	cfg   *config.ClientConfig
	creds Credentials
	ks    killSwitch

	mu        sync.Mutex
	state     State
	vpnIP     string
	gateway   string
	lastError string // 最近一次对用户可见的错误（杀开关失败等）
	ksOK      bool   // 杀开关当前是否已成功启用（无杀开关配置时为 true）
	rt        *runtime
	reconnect *transport.ReconnectClient
	cancel    context.CancelFunc
	runCtx    context.Context

	activeMu    sync.Mutex
	activeConn  *transport.Conn
	cryptoSess  *crypto.Session
	sessionPriv string // 当前会话私钥（握手下发或配置）

	// clearRoutesHook 仅单测：在 clearRoutes 前调用，用于验证杀开关顺序。
	clearRoutesHook func()
}

// NewEngine 创建尚未连接的客户端 VPN 引擎实例。
//
// 参数：cfg — 客户端配置；调用方保证生命周期内有效。
// 返回：state=StateIdle 的 Engine；须 SetCredentials 后 Start。
func NewEngine(cfg *config.ClientConfig) *Engine {
	return &Engine{cfg: cfg, state: StateIdle, rt: &runtime{cfg: cfg}, ks: netstackKillSwitch{}, ksOK: true}
}

// protectThenClearRoutes 断线防护：先装杀开关再清路由；Enable 失败则禁止清路由，避免明文绕行。
func (e *Engine) protectThenClearRoutes() {
	if e.cfg.Security.KillSwitch {
		prefixes := e.rt.allowedIPs()
		if len(prefixes) == 0 {
			e.setKillSwitchStatus(false, "杀开关启用失败: AllowedIPs 为空，已保留路由以防泄漏")
			logger.Error("杀开关启用失败: AllowedIPs 为空，禁止清路由")
			return
		}
		if err := e.ks.Enable(prefixes); err != nil {
			e.setKillSwitchStatus(false, fmt.Sprintf("杀开关启用失败: %v（已保留路由，工控流量仍走 TUN）", err))
			logger.Error("杀开关启用失败，禁止清路由: %v", err)
			return
		}
		e.setKillSwitchStatus(true, "")
	}
	if e.clearRoutesHook != nil {
		e.clearRoutesHook()
	}
	e.rt.clearRoutes()
}

// setKillSwitchStatus 更新杀开关可见状态（供 GUI）。
func (e *Engine) setKillSwitchStatus(ok bool, userErr string) {
	e.mu.Lock()
	e.ksOK = ok
	e.lastError = userErr
	e.mu.Unlock()
}

// SetCredentials 设置或更新隧道登录凭据（如 GUI 登录/退出后重填）。
//
// 参数：c — 账号密码必填；PrivateKey 可选，握手未下发私钥时作回退。
// 返回：无。
// 副作用：覆盖 Engine 内 creds；不影响已连接会话，下次 onConnect 使用新凭据。
// 并发：持 mu 锁；可与 State 等查询并行，但与 Start/Stop 同 goroutine 调用为宜。
func (e *Engine) SetCredentials(c Credentials) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.creds = c
}

// State 返回当前连接状态（持 mu 锁读取）。
//
// 并发：可与 LastError 等查询并行；Start/Stop 应由单 goroutine 串行调用。
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// LastError 返回最近一次对用户可见的错误文案。
//
// 空字符串表示无错误；杀开关失败等场景由 setKillSwitchStatus 写入。
func (e *Engine) LastError() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastError
}

// KillSwitchOK 返回杀开关是否处于预期保护状态。
//
// 未开启 kill_switch 配置时恒为 true；断线后 Enable 失败时为 false。
func (e *Engine) KillSwitchOK() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.cfg.Security.KillSwitch {
		return true
	}
	return e.ksOK
}

// VPNIP 返回握手成功后分配的虚拟 IP。
//
// 未连接、重连中或 Stop 后为空字符串。
func (e *Engine) VPNIP() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vpnIP
}

// Start 在后台启动 TLS 重连循环与 TUN 读循环；重复调用若已在运行则直接返回 nil。
//
// 参数：无（配置与凭据来自 NewEngine/SetCredentials）。
// 返回：err — 杀开关平台不支持或 BuildClientTLS 失败；已在运行时为 nil。
// 副作用：创建 runCtx/cancel、ReconnectClient、tunReadLoop goroutine；state 变为 StateConnecting。
// 并发：非阻塞；重连与 onConnect 在 transport 内部 goroutine 执行；调用方不应并行多次 Start。
func (e *Engine) Start() error {
	if e.cfg.Security.KillSwitch {
		if err := e.ks.Supported(); err != nil {
			return fmt.Errorf("杀开关: %w", err)
		}
	}
	tlsCfg, err := security.BuildClientTLS(e.cfg)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.runCtx = ctx
	e.cancel = cancel
	e.state = StateConnecting
	e.mu.Unlock()

	tcfg := transport.FromClientConfig(e.cfg)
	e.reconnect = transport.NewReconnectClient(e.cfg.Server.Address, tlsCfg, tcfg, nil, e.onConnect)
	e.reconnect.Start()

	safeutil.GoSafe("client-tun-read", func() {
		e.tunReadLoop(ctx)
	})
	return nil
}

// Stop 停止重连循环、关闭 TUN/路由并拆除杀开关（退出登录或进程退出时调用）。
//
// 参数：无。
// 返回：无。
// 副作用：清空 activeConn/cryptoSess、cancel 上下文、Stop reconnect、rt.close、ks.Remove；
//         state 置 StateIdle，vpnIP 清空；Remove 失败时写入 lastError。
// 并发：可阻塞至 reconnect 停止；锁序 activeMu → mu；与 Start 不可并行，应由同一控制 goroutine 调用。
func (e *Engine) Stop() {
	// 锁序：activeMu → mu，与 onClose 一致，避免死锁。
	e.activeMu.Lock()
	e.activeConn = nil
	e.cryptoSess = nil
	e.sessionPriv = ""
	e.activeMu.Unlock()

	e.mu.Lock()
	cancel := e.cancel
	e.cancel = nil
	rc := e.reconnect
	e.reconnect = nil
	e.state = StateIdle
	e.vpnIP = ""
	e.mu.Unlock()

	if rc != nil {
		rc.Stop()
	}
	if cancel != nil {
		cancel()
	}
	e.rt.close()
	if err := e.ks.Remove(); err != nil {
		logger.Error("拆除杀开关失败: %v", err)
		e.setKillSwitchStatus(false, fmt.Sprintf("拆除杀开关失败: %v", err))
	} else {
		e.setKillSwitchStatus(true, "")
	}
}

func (e *Engine) setState(st State) {
	e.mu.Lock()
	e.state = st
	e.mu.Unlock()
}

// onConnect 由 transport.ReconnectClient 在每次 TLS 连接建立后回调，完成握手、策略与数据面绑定。
//
// 参数：conn — 新建立的 TLS 连接；失败或放弃时由本函数 Close。
// 返回：无（错误路径 Close conn 并可能 protectThenClearRoutes）。
// 副作用：隧道鉴权、建立 cryptoSess、rt.applyPolicy（TUN/路由/DNS）、注册 OnData/OnClose；
//         成功时 state=StateConnected 并可能 Disable 杀开关；失败时清路由/杀开关保护。
// 并发：在 transport 重连 goroutine 中运行；与 tunReadLoop 通过 activeMu 协调 activeConn/cryptoSess。
func (e *Engine) onConnect(conn *transport.Conn) {
	e.setState(StateConnecting)
	e.mu.Lock()
	creds := e.creds
	e.mu.Unlock()

	// --- 阶段 1：隧道账号密码握手 ---
	hs := tunnel.NewClientHandshake()
	user := strings.TrimSpace(creds.Username)
	pass := creds.Password
	if user == "" || pass == "" {
		logger.Warn("隧道握手失败: 缺少账号密码")
		conn.Close()
		return
	}
	hsStart := time.Now()
	hsRes, err := hs.RunAuthWithTimeout(conn, user, pass, 20*time.Second)
	if err != nil {
		logger.Warn("隧道握手失败: %v elapsed=%s", err, time.Since(hsStart))
		conn.Close()
		return
	}
	logger.Info("隧道鉴权应答收到 elapsed=%s", time.Since(hsStart))

	// --- 阶段 2：建立隧道加密会话（私钥优先用握手下发） ---
	priv := strings.TrimSpace(hsRes.ClientPrivateKey)
	if priv == "" {
		priv = strings.TrimSpace(creds.PrivateKey)
	}
	if priv == "" {
		logger.Warn("握手未下发私钥且无内存回退私钥")
		conn.Close()
		return
	}
	sess, err := tunnel.BuildClientCrypto(priv, hsRes.ServerPublicKey)
	if err != nil {
		logger.Warn("建立加密会话失败: %v", err)
		conn.Close()
		return
	}

	conn.SetOnData(func(data []byte) {
		plain, err := sess.Decrypt(data)
		if err != nil {
			return
		}
		_ = e.rt.write(plain)
	})
	conn.SetOnClose(func(error) {
		e.activeMu.Lock()
		defer e.activeMu.Unlock()
		if e.activeConn != conn {
			return
		}
		e.activeConn = nil
		e.cryptoSess = nil
		logger.Info("隧道连接已断开，等待重连")
		e.protectThenClearRoutes()
		e.mu.Lock()
		e.state = StateReconnecting
		e.mu.Unlock()
	})
	e.activeMu.Lock()
	e.cryptoSess = sess
	e.activeConn = conn
	e.sessionPriv = priv
	e.activeMu.Unlock()

	// --- 阶段 3：按握手策略开 TUN/路由/DNS；成功后再拆杀开关 ---
	policyStart := time.Now()
	if err := e.rt.applyPolicy(hsRes.Policy); err != nil {
		logger.Warn("应用服务端策略失败: %v elapsed=%s", err, time.Since(policyStart))
		e.protectThenClearRoutes()
		conn.Close()
		return
	}

	e.activeMu.Lock()
	stillActive := e.activeConn == conn && conn.State() == transport.StateConnected
	if !stillActive {
		if e.activeConn == conn {
			e.activeConn = nil
			e.cryptoSess = nil
		}
		e.activeMu.Unlock()
		logger.Warn("session_abandoned reason=disconnected_during_policy")
		e.protectThenClearRoutes()
		return
	}
	mtu := netutil.ResolveMTU(hsRes.Policy.MTU, e.cfg.Tun.MTU)
	e.mu.Lock()
	e.vpnIP = hsRes.Policy.VPNIP
	e.gateway = hsRes.Policy.GatewayIP
	e.state = StateConnected
	e.mu.Unlock()
	e.activeMu.Unlock()

	if e.cfg.Security.KillSwitch {
		if err := e.ks.Disable(); err != nil {
			logger.Error("杀开关拆除失败: %v", err)
			e.setKillSwitchStatus(false, fmt.Sprintf("杀开关拆除失败: %v", err))
		} else {
			e.setKillSwitchStatus(true, "")
		}
	}
	logger.Info("隧道握手成功 vpn_ip=%s policy_ver=%d gateway=%s mtu=%d policy_elapsed=%s",
		hsRes.Policy.VPNIP, hsRes.Policy.PolicyVer, hsRes.Policy.GatewayIP, mtu, time.Since(policyStart))
}

func (e *Engine) tunReadLoop(ctx context.Context) {
	mtu := netutil.ResolveMTU(e.cfg.Tun.MTU)
	e.rt.readLoop(ctx, func(b []byte) error {
		e.activeMu.Lock()
		conn := e.activeConn
		sess := e.cryptoSess
		e.activeMu.Unlock()
		if conn == nil || sess == nil {
			return nil
		}
		if conn.State() != transport.StateConnected {
			return nil
		}
		enc, err := sess.Encrypt(b)
		if err != nil {
			return err
		}
		return conn.Send(enc)
	}, mtu)
}

// PromptPassword 从终端读取密码（无回显需调用方处理；此处简单 Scanln）。
func PromptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "请输入密码（或设置 HAOVPN_PASSWORD）: ")
	var s string
	_, err := fmt.Scanln(&s)
	return s, err
}
