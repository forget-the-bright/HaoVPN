package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"haovpn/internal/brand"
	"haovpn/internal/netutil"
)
// ClientConfig 客户端完整配置（对应 client.yaml），CLI/GUI 与服务模式共用。
//
// 各 YAML 段职责：
//   Server — 隧道 TLS 连接目标与传输计时；
//   Tun — 本地 Wintun/TUN 接口参数；
//   Auth — Web/隧道账号密码（优先于纯私钥模式）；
//   Peer — 密钥与 yaml 兼容分流项（运行时以握手推送为准）；
//   Security — Kill-switch 等客户端安全选项；
//   Reconnect — 断线指数退避；
//   Log — 日志级别与滚动文件路径。
//
// ApplyDefaults 填充缺省；Validate 在连接前校验；握手可覆盖 Peer 段 vpn_ip、gateway、allowed_ips 等。
type ClientConfig struct {
	Server    ClientServerSection    `yaml:"server"`    // 服务端地址、TLS、传输心跳/拨号超时
	Tun       ClientTunSection       `yaml:"tun"`       // 本地 TUN 网卡名、MTU、DNS 策略
	Auth      ClientAuthSection      `yaml:"auth"`      // Web/隧道账号密码（优先于纯私钥）
	Peer      ClientPeerSection      `yaml:"peer"`      // 密钥与 yaml 兼容分流（握手优先）
	Security  ClientSecuritySection  `yaml:"security"`  // Kill-switch 等客户端安全选项
	Reconnect ReconnectSection       `yaml:"reconnect"` // 断线指数退避重连
	Log       LogSection             `yaml:"log"`       // 日志级别与文件路径
}

// ClientServerSection 要连接的服务端地址、TLS 与传输层心跳/拨号超时。
//
// 字段默认值由 ApplyDefaults 填充（心跳/拨号见 netutil 常量）；握手不下发本段。
type ClientServerSection struct {
	Address              string           `yaml:"address"`                // host:port；Validate 必填
	TLS                  ClientTLSSection `yaml:"tls"`                    // CA 与证书校验选项
	HeartbeatIntervalSec int              `yaml:"heartbeat_interval_sec"` // 心跳发送间隔秒；默认 netutil.DefaultHeartbeatIntervalSec
	HeartbeatTimeoutSec  int              `yaml:"heartbeat_timeout_sec"`  // 对端静默超时秒；默认 netutil.DefaultHeartbeatTimeoutSec
	DialTimeoutSec       int              `yaml:"dial_timeout_sec"`       // TCP 拨号超时秒；默认 3
}

// ClientTLSSection 客户端 TLS 校验选项（连接 server.address 时使用）。
//
// 生产环境须配置 ca_file；insecure_skip_verify 仅用于开发自签场景。
type ClientTLSSection struct {
	CAFile             string `yaml:"ca_file"`              // 服务端 CA PEM；非 insecure 时 Validate 必填
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // true 跳过证书校验（仅开发）
	ServerName         string `yaml:"server_name"`          // TLS SNI/SAN 校验名；空则从 address 推导
}

// ClientTunSection 本地 Wintun/TUN 网卡参数。
//
// dns_from_policy 默认 true：连接成功后应用握手推送的 DNS；false 则不改系统 DNS。
type ClientTunSection struct {
	Name          string `yaml:"name"`            // 网卡名；默认 brand.DefaultTunName
	MTU           int    `yaml:"mtu"`             // 接口 MTU；默认 netutil.DefaultMTU
	DNSFromPolicy *bool  `yaml:"dns_from_policy"` // 默认 true：应用握手推送的 DNS 服务器
}

// DNSFromPolicyEnabled 是否应用握手下发的 DNS 服务器列表。
//
// 返回：dns_from_policy 未配置（nil）时默认 true。
func (t ClientTunSection) DNSFromPolicyEnabled() bool {
	if t.DNSFromPolicy == nil {
		return true
	}
	return *t.DNSFromPolicy
}

// ClientSecuritySection 客户端安全选项（Windows WFP 杀开关等）。
//
// kill_switch 默认 false；true 时断线阻断 AllowedIPs 出站（须 Windows）。
type ClientSecuritySection struct {
	KillSwitch bool `yaml:"kill_switch"` // 断线时阻断 AllowedIPs 外出站；默认 false
}

// ClientAuthSection 隧道/Web 登录凭据。
//
// Validate 要求 username 非空（或环境变量 HAOVPN_USER）；密码推荐 HAOVPN_PASSWORD 而非写进 yaml。
type ClientAuthSection struct {
	Username string `yaml:"username"` // 登录名；Validate 要求非空（或 HAOVPN_USER）
	Password string `yaml:"password"` // 可选；更推荐环境变量 HAOVPN_PASSWORD
}

// ClientPeerSection VPN 身份与 yaml 兼容分流项。
//
// 账号密码模式下 private_key 由服务端下发；运行时 allowed_ips、vpn_ip、gateway 以握手为准。
type ClientPeerSection struct {
	PrivateKey      string   `yaml:"private_key"`       // 客户端私钥；账号密码模式下由服务端下发
	PublicKey       string   `yaml:"public_key"`        // 客户端公钥；可选，通常由私钥推导
	ServerPublicKey string   `yaml:"server_public_key"` // 服务端公钥；握手可覆盖
	VPNIP           string   `yaml:"vpn_ip"`            // yaml 静态 IP；动态账号由握手分配
	GatewayIP       string   `yaml:"gateway_ip"`        // 路由下一跳；优先握手 gateway_ip
	AllowedIPs      []string `yaml:"allowed_ips"`       // yaml 分流；握手 allowed_ips 覆盖
}

// ReconnectSection 断线后指数退避重连参数（映射 transport.Config）。
//
// initial_sec / max_sec 缺省见 netutil.DefaultReconnectInitialSec / MaxSec。
type ReconnectSection struct {
	InitialSec int `yaml:"initial_sec"` // 首次重试间隔秒；默认 netutil.DefaultReconnectInitialSec
	MaxSec     int `yaml:"max_sec"`     // 退避上限秒；默认 netutil.DefaultReconnectMaxSec
}

// ApplyDefaults 填充客户端 YAML 缺省项。
//
// 可安全重复调用；GUI 内存配置与 Validate 前均应调用。
func (c *ClientConfig) ApplyDefaults() {
	if strings.TrimSpace(c.Tun.Name) == "" {
		c.Tun.Name = brand.DefaultTunName
	}
	if c.Tun.MTU <= 0 {
		c.Tun.MTU = netutil.DefaultMTU
	}
	if c.Reconnect.InitialSec <= 0 {
		c.Reconnect.InitialSec = netutil.DefaultReconnectInitialSec
	}
	if c.Reconnect.MaxSec <= 0 {
		c.Reconnect.MaxSec = netutil.DefaultReconnectMaxSec
	}
	if c.Server.HeartbeatIntervalSec <= 0 {
		c.Server.HeartbeatIntervalSec = netutil.DefaultHeartbeatIntervalSec
	}
	if c.Server.HeartbeatTimeoutSec <= 0 {
		c.Server.HeartbeatTimeoutSec = netutil.DefaultHeartbeatTimeoutSec
	}
	if c.Server.DialTimeoutSec <= 0 {
		c.Server.DialTimeoutSec = netutil.DefaultDialTimeoutSec
	}
	if strings.TrimSpace(c.Log.Level) == "" {
		c.Log.Level = "info"
	}
	if strings.TrimSpace(c.Log.File) == "" {
		c.Log.File = "./logs/client.log"
	}
}

// Validate 校验客户端配置必填项与安全策略。
//
// 要求 server.address、auth.username、TLS CA（或非 insecure）；校验 peer.allowed_ips 非全隧道。
// 返回：字段名含中文说明的 error。
func (c *ClientConfig) Validate() error {
	c.ApplyDefaults()
	if strings.TrimSpace(c.Server.Address) == "" {
		return fmt.Errorf("配置错误: server.address 不能为空")
	}
	if _, _, err := net.SplitHostPort(strings.TrimSpace(c.Server.Address)); err != nil {
		return fmt.Errorf("配置错误: server.address 须为 host:port: %w", err)
	}
	hasAuthUser := strings.TrimSpace(c.Auth.Username) != ""
	if !hasAuthUser {
		return fmt.Errorf("配置错误: 须配置 auth.username（账号密码登录）")
	}
	if err := netutil.ValidateNoFullTunnel(c.Peer.AllowedIPs); err != nil {
		return fmt.Errorf("配置错误: peer.allowed_ips %w", err)
	}
	if !c.Server.TLS.InsecureSkipVerify && strings.TrimSpace(c.Server.TLS.CAFile) == "" {
		return fmt.Errorf("配置错误: 须配置 server.tls.ca_file，或显式 insecure_skip_verify: true")
	}
	return nil
}

// ResolveAuth 解析登录账号与密码。
//
// 优先级：yaml auth > 环境变量 HAOVPN_USER / HAOVPN_PASSWORD。
func (c *ClientConfig) ResolveAuth() (username, password string) {
	username = strings.TrimSpace(c.Auth.Username)
	password = c.Auth.Password
	if username == "" {
		username = strings.TrimSpace(os.Getenv(brand.EnvUser))
	}
	if password == "" {
		password = os.Getenv(brand.EnvPassword)
	}
	return username, password
}

// ResolveGatewayFor 解析路由下一跳：yaml gateway_ip 优先，否则按 vpnIP 推断 .1。
//
// 参数：vpnIP 为空时使用本段 VPNIP 字段。
func (p *ClientPeerSection) ResolveGatewayFor(vpnIP string) string {
	ip := vpnIP
	if strings.TrimSpace(ip) == "" {
		ip = p.VPNIP
	}
	return netutil.ResolveGateway("", p.GatewayIP, ip)
}

// PreferGateway 合并握手与 yaml 网关：握手 gateway_ip > yaml > InferGatewayFromVPNIP。
func PreferGateway(handshakeGW, vpnIP string, peer *ClientPeerSection) string {
	yamlGW := ""
	if peer != nil {
		yamlGW = peer.GatewayIP
	}
	return netutil.ResolveGateway(handshakeGW, yamlGW, vpnIP)
}
