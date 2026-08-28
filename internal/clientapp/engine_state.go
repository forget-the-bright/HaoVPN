package clientapp

import (
	"context"
	"sync"

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
	rt        *runtime
	reconnect *transport.ReconnectClient
	cancel    context.CancelFunc
	runCtx    context.Context

	activeMu    sync.Mutex
	activeConn  *transport.Conn
	cryptoSess  *crypto.Session
	sessionPriv string

	clearRoutesHook func()
}

// NewEngine 创建尚未连接的客户端 VPN 引擎实例。
func NewEngine(cfg *config.ClientConfig) *Engine {
	return &Engine{cfg: cfg, state: StateIdle, rt: &runtime{cfg: cfg}, ks: netstackKillSwitch{}, ksOK: true}
}

// SetCredentials 设置或更新隧道登录凭据。
func (e *Engine) SetCredentials(c Credentials) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.creds = c
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

func (e *Engine) setState(st State) {
	e.mu.Lock()
	e.state = st
	e.mu.Unlock()
}
