package config

import (
	"fmt"
	"net"
	"os"
	"strings"

	"haovpn/internal/brand"
)

// ClientConfig 客户端完整配置结构（对应 client.yaml）
type ClientConfig struct {
	Server    ClientServerSection    `yaml:"server"`
	Tun       ClientTunSection       `yaml:"tun"`
	Auth      ClientAuthSection      `yaml:"auth"`
	Peer      ClientPeerSection      `yaml:"peer"`
	Security  ClientSecuritySection  `yaml:"security"`
	Reconnect ReconnectSection       `yaml:"reconnect"`
	Log       LogSection             `yaml:"log"`
}

// ClientServerSection 要连接的服务端地址与 TLS
type ClientServerSection struct {
	Address              string           `yaml:"address"`
	TLS                  ClientTLSSection `yaml:"tls"`
	HeartbeatIntervalSec int              `yaml:"heartbeat_interval_sec"`
	HeartbeatTimeoutSec  int              `yaml:"heartbeat_timeout_sec"`
	DialTimeoutSec       int              `yaml:"dial_timeout_sec"` // TCP 拨号超时秒；默认 3
}

// ClientTLSSection 客户端 TLS 校验选项
type ClientTLSSection struct {
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	ServerName         string `yaml:"server_name"` // 校验证书 SAN 用，默认可从 address 推导
}

// ClientTunSection 本地 TUN 网卡
type ClientTunSection struct {
	Name          string `yaml:"name"`
	MTU           int    `yaml:"mtu"`
	DNSFromPolicy *bool  `yaml:"dns_from_policy"` // 默认 true：应用握手推送 DNS
}

// DNSFromPolicyEnabled 是否应用握手下发的 DNS（未配置时默认 true）。
func (t ClientTunSection) DNSFromPolicyEnabled() bool {
	if t.DNSFromPolicy == nil {
		return true
	}
	return *t.DNSFromPolicy
}

// ClientSecuritySection 客户端安全选项。
type ClientSecuritySection struct {
	KillSwitch bool `yaml:"kill_switch"` // 断线时阻断 AllowedIPs 出站（Windows）
}

// ClientAuthSection 隧道账号密码（优先于仅私钥旧模式）。
type ClientAuthSection struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"` // 可选；更推荐环境变量 HAOVPN_PASSWORD
}

// ClientPeerSection VPN 身份与分流（策略字段由握手下发，yaml 可选兼容）。
type ClientPeerSection struct {
	PrivateKey      string   `yaml:"private_key"`
	PublicKey       string   `yaml:"public_key"`
	ServerPublicKey string   `yaml:"server_public_key"`
	VPNIP           string   `yaml:"vpn_ip"`
	GatewayIP       string   `yaml:"gateway_ip"` // 可选兼容；优先握手 gateway_ip
	AllowedIPs      []string `yaml:"allowed_ips"`
}

// ResolveGateway 返回客户端路由用的 VPN 网关 IP。
func (p *ClientPeerSection) ResolveGateway() string {
	if strings.TrimSpace(p.GatewayIP) != "" {
		return strings.TrimSpace(p.GatewayIP)
	}
	return inferGatewayFromVPNIP(p.VPNIP)
}

func inferGatewayFromVPNIP(vpnIP string) string {
	ip := net.ParseIP(vpnIP)
	if ip == nil {
		return "10.88.0.1"
	}
	v4 := ip.To4()
	if v4 == nil {
		return "10.88.0.1"
	}
	return fmt.Sprintf("%d.%d.%d.1", v4[0], v4[1], v4[2])
}

// ReconnectSection 断线指数退避重连参数
type ReconnectSection struct {
	InitialSec int `yaml:"initial_sec"`
	MaxSec     int `yaml:"max_sec"`
}

// Validate 校验客户端配置。
// 须有 server.address；鉴权为「auth 用户名」或「peer.private_key」二选一（密码可稍后交互补齐）。
func (c *ClientConfig) Validate() error {
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
	for _, cidr := range c.Peer.AllowedIPs {
		if cidr == "0.0.0.0/0" || cidr == "::/0" {
			return fmt.Errorf("配置错误: peer.allowed_ips 禁止全隧道 0.0.0.0/0")
		}
	}
	if c.Tun.MTU <= 0 {
		c.Tun.MTU = 1420
	}
	if c.Reconnect.InitialSec <= 0 {
		c.Reconnect.InitialSec = 1
	}
	if c.Reconnect.MaxSec <= 0 {
		c.Reconnect.MaxSec = 3
	}
	if c.Server.HeartbeatIntervalSec <= 0 {
		c.Server.HeartbeatIntervalSec = 15
	}
	if c.Server.HeartbeatTimeoutSec <= 0 {
		c.Server.HeartbeatTimeoutSec = 90
	}
	if c.Server.DialTimeoutSec <= 0 {
		c.Server.DialTimeoutSec = 3
	}
	if !c.Server.TLS.InsecureSkipVerify && strings.TrimSpace(c.Server.TLS.CAFile) == "" {
		return fmt.Errorf("配置错误: 须配置 server.tls.ca_file，或显式 insecure_skip_verify: true")
	}
	return nil
}

// ResolveAuth 解析登录账号：yaml auth > 环境变量 HAOVPN_USER / HAOVPN_PASSWORD。
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

// ResolveGatewayFor 路由下一跳：优先 yaml gateway_ip，否则按 vpnIP 推断 .1。
// 握手下发的 gateway_ip 应由调用方直接使用，不必再经此函数。
func (p *ClientPeerSection) ResolveGatewayFor(vpnIP string) string {
	if strings.TrimSpace(p.GatewayIP) != "" {
		return strings.TrimSpace(p.GatewayIP)
	}
	if strings.TrimSpace(vpnIP) != "" {
		return inferGatewayFromVPNIP(vpnIP)
	}
	return inferGatewayFromVPNIP(p.VPNIP)
}

// PreferGateway 优先使用握手网关，否则回退 ResolveGatewayFor。
func PreferGateway(handshakeGW, vpnIP string, peer *ClientPeerSection) string {
	if strings.TrimSpace(handshakeGW) != "" {
		return strings.TrimSpace(handshakeGW)
	}
	if peer == nil {
		return inferGatewayFromVPNIP(vpnIP)
	}
	return peer.ResolveGatewayFor(vpnIP)
}
