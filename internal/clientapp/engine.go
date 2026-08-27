// Package clientapp 提供 CLI/GUI 共用的客户端拨号、策略应用与重连引擎。
package clientapp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netstack"
	"haovpn/internal/safeutil"
	"haovpn/internal/security"
	"haovpn/internal/transport"
	"haovpn/internal/tunnel"
	"haovpn/internal/tun"
)

// State 连接状态（供 GUI 展示）。
type State int

const (
	StateIdle State = iota
	StateConnecting
	StateConnected
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

// Credentials 隧道登录凭据。
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

// Engine 客户端隧道引擎。
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

// NewEngine 创建引擎（尚未连接）。
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

// SetCredentials 设置/更新登录凭据（退出登录后重填）。
func (e *Engine) SetCredentials(c Credentials) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.creds = c
}

// State 当前状态。
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// LastError 最近一次对用户可见的错误（空表示无）。
func (e *Engine) LastError() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastError
}

// KillSwitchOK 杀开关是否处于已保护状态（未开启杀开关时恒为 true）。
func (e *Engine) KillSwitchOK() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.cfg.Security.KillSwitch {
		return true
	}
	return e.ksOK
}

// VPNIP 已分配的虚拟 IP（未连接为空）。
func (e *Engine) VPNIP() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.vpnIP
}

// Start 后台启动重连循环；重复调用无效。
func (e *Engine) Start() error {
	if e.cfg.Security.KillSwitch {
		if err := e.ks.Supported(); err != nil {
			return fmt.Errorf("杀开关: %w", err)
		}
	}
	tlsCfg, err := BuildClientTLS(e.cfg)
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

	tcfg := transport.DefaultConfig()
	tcfg.ReconnectInitial = time.Duration(e.cfg.Reconnect.InitialSec) * time.Second
	tcfg.ReconnectMax = time.Duration(e.cfg.Reconnect.MaxSec) * time.Second
	if e.cfg.Server.DialTimeoutSec > 0 {
		tcfg.DialTimeout = time.Duration(e.cfg.Server.DialTimeoutSec) * time.Second
	}
	if e.cfg.Server.HeartbeatIntervalSec > 0 {
		tcfg.HeartbeatInterval = time.Duration(e.cfg.Server.HeartbeatIntervalSec) * time.Second
	}
	if e.cfg.Server.HeartbeatTimeoutSec > 0 {
		tcfg.HeartbeatTimeout = time.Duration(e.cfg.Server.HeartbeatTimeoutSec) * time.Second
	}

	e.reconnect = transport.NewReconnectClient(e.cfg.Server.Address, tlsCfg, tcfg, nil, e.onConnect)
	e.reconnect.Start()

	safeutil.GoSafe("client-tun-read", func() {
		e.tunReadLoop(ctx)
	})
	return nil
}

// Stop 停止重连、关闭 TUN（退出登录/退出程序）。
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

func (e *Engine) onConnect(conn *transport.Conn) {
	e.setState(StateConnecting)
	e.mu.Lock()
	creds := e.creds
	e.mu.Unlock()

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

	priv := strings.TrimSpace(hsRes.ClientPrivateKey)
	if priv == "" {
		priv = strings.TrimSpace(creds.PrivateKey)
	}
	if priv == "" {
		logger.Warn("握手未下发私钥且配置无 private_key")
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
	mtu := hsRes.Policy.MTU
	if mtu <= 0 {
		mtu = e.cfg.Tun.MTU
	}
	if mtu <= 0 {
		mtu = 1420
	}
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
	mtu := e.cfg.Tun.MTU
	if mtu <= 0 {
		mtu = 1420
	}
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

// runtime TUN 与路由。
type runtime struct {
	mu         sync.Mutex
	cfg        *config.ClientConfig
	tunDev     tun.Device
	routes     []string
	allowedCIDRs []string
	vpnIP      string
	policyVer  int
	gateway    string
}

func (rt *runtime) allowedIPs() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string{}, rt.allowedCIDRs...)
}

func (rt *runtime) applyPolicy(policy tunnel.HandshakePolicy) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if policy.VPNIP == "" {
		return fmt.Errorf("握手未下发 vpn_ip")
	}
	if rt.cfg.Peer.VPNIP != "" && rt.cfg.Peer.VPNIP != policy.VPNIP {
		logger.Warn("配置 vpn_ip=%s 与握手应答 %s 不一致，以服务端为准", rt.cfg.Peer.VPNIP, policy.VPNIP)
	}
	if len(rt.cfg.Peer.AllowedIPs) > 0 {
		logger.Warn("配置 allowed_ips 将被握手应答覆盖（服务端权威）")
	}

	mtu := policy.MTU
	if mtu <= 0 {
		mtu = rt.cfg.Tun.MTU
	}
	if mtu <= 0 {
		mtu = 1420
	}

	needRecreate := rt.tunDev == nil || rt.vpnIP != policy.VPNIP
	if needRecreate {
		if rt.tunDev != nil {
			rt.clearRoutesLocked() // 内含一次 RestoreDNS
			_ = rt.tunDev.Close()
			rt.tunDev = nil
		}
		dev, err := tun.Open(tun.Config{
			Name: rt.cfg.Tun.Name,
			MTU:  mtu,
			CIDR: policy.VPNIP + "/32",
		})
		if err != nil {
			return fmt.Errorf("TUN 创建失败: %w", err)
		}
		rt.tunDev = dev
		rt.vpnIP = policy.VPNIP
		rt.routes = nil
	} else {
		rt.clearRoutesLocked()
	}

	gw := config.PreferGateway(policy.GatewayIP, policy.VPNIP, &rt.cfg.Peer)
	rt.gateway = gw
	tunName := rt.tunDev.Name()
	if gw != "" {
		gwCIDR := gw + "/32"
		if err := netstack.AddClientRoute(gwCIDR, tunName, gw); err != nil {
			logger.Warn("添加网关路由 %s: %v", gwCIDR, err)
		} else {
			rt.routes = append(rt.routes, gwCIDR)
		}
	}
	for _, cidr := range policy.AllowedIPs {
		if gw != "" && cidr == gw+"/32" {
			continue
		}
		if err := netstack.AddClientRoute(cidr, tunName, gw); err != nil {
			logger.Warn("添加路由 %s: %v", cidr, err)
		} else {
			rt.routes = append(rt.routes, cidr)
		}
	}

	if policy.PolicyVer != rt.policyVer && rt.policyVer > 0 {
		logger.Info("策略已更新 policy_ver %d -> %d", rt.policyVer, policy.PolicyVer)
	}
	rt.policyVer = policy.PolicyVer
	rt.allowedCIDRs = append([]string{}, policy.AllowedIPs...)
	logger.Info("已应用服务端策略 vpn_ip=%s allowed_ips=%v policy_ver=%d gateway=%s mtu=%d",
		policy.VPNIP, policy.AllowedIPs, policy.PolicyVer, gw, mtu)

	if rt.cfg.Tun.DNSFromPolicyEnabled() && len(policy.DNSServers) > 0 {
		if err := netstack.ApplyDNS(tunName, policy.DNSServers); err != nil {
			// 非 Windows 等平台会返回明确错误；禁止打 dns_applied 假装成功。
			logger.Warn("DNS 设置失败（未应用）adapter=%s: %v", tunName, err)
		} else {
			logger.Info("dns_applied servers=%v adapter=%s", policy.DNSServers, tunName)
		}
	}
	return nil
}

func (rt *runtime) clearRoutes() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
}

func (rt *runtime) clearRoutesLocked() {
	if rt.tunDev == nil {
		return
	}
	tunName := rt.tunDev.Name()
	gw := rt.gateway
	if gw == "" {
		gw = rt.cfg.Peer.ResolveGatewayFor(rt.vpnIP)
	}
	for _, cidr := range rt.routes {
		_ = netstack.DelClientRoute(cidr, tunName, gw)
	}
	rt.routes = nil
	_ = netstack.RestoreDNS(tunName)
}

func (rt *runtime) close() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.clearRoutesLocked()
	if rt.tunDev != nil {
		_ = rt.tunDev.Close()
		rt.tunDev = nil
	}
	rt.allowedCIDRs = nil
}

func (rt *runtime) write(pkt []byte) error {
	rt.mu.Lock()
	dev := rt.tunDev
	rt.mu.Unlock()
	if dev == nil {
		return nil
	}
	_, err := dev.Write(pkt)
	return err
}

func (rt *runtime) readLoop(ctx context.Context, send func([]byte) error, mtu int) {
	buf := make([]byte, mtu+100)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		rt.mu.Lock()
		dev := rt.tunDev
		rt.mu.Unlock()
		if dev == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		n, err := dev.Read(buf)
		if err != nil {
			logger.Warn("TUN 读错误: %v", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err := send(buf[:n]); err != nil {
			logger.Warn("隧道发送失败: %v", err)
		}
	}
}

// BuildClientTLS 构造客户端 TLS 配置；CA 无效时返回错误（不静默回退）。
func BuildClientTLS(cfg *config.ClientConfig) (*tls.Config, error) {
	tlsCfg := security.TLSConfig(tls.Certificate{}, false)
	tlsCfg.InsecureSkipVerify = cfg.Server.TLS.InsecureSkipVerify
	if !cfg.Server.TLS.InsecureSkipVerify {
		ca := strings.TrimSpace(cfg.Server.TLS.CAFile)
		if ca == "" {
			return nil, fmt.Errorf("未配置 server.tls.ca_file 且未启用 insecure_skip_verify")
		}
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 失败 %s: %w", ca, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("CA 文件不是有效 PEM: %s", ca)
		}
		tlsCfg.RootCAs = pool
	} else if cfg.Server.TLS.CAFile != "" {
		pem, err := os.ReadFile(cfg.Server.TLS.CAFile)
		if err == nil {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM(pem) {
				tlsCfg.RootCAs = pool
			}
		}
	}
	tlsCfg.ServerName = cfg.Server.TLS.ServerName
	if tlsCfg.ServerName == "" {
		host, _, err := net.SplitHostPort(cfg.Server.Address)
		if err != nil {
			host = cfg.Server.Address
		}
		host = strings.Trim(host, "[]")
		if host != "" && host != "0.0.0.0" && !strings.HasPrefix(host, "REPLACE_") {
			tlsCfg.ServerName = host
		} else {
			tlsCfg.ServerName = "localhost"
		}
	}
	return tlsCfg, nil
}

// PromptPassword 从终端读取密码（无回显需调用方处理；此处简单 Scanln）。
func PromptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "密码: ")
	var s string
	_, err := fmt.Scanln(&s)
	return s, err
}
