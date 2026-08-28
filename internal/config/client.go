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
//   Auth — 隧道账号密码（remember_password 可选明文写回）；
//   Security — Kill-switch 等客户端安全选项；
//   Reconnect — 断线指数退避；
//   Log — 日志级别与滚动文件路径。
//
// vpn_ip / allowed_ips / gateway / 私钥均由握手下发，client.yaml 不含 peer 段。
// ApplyDefaults 填充缺省；Validate 在连接前校验。
type ClientConfig struct {
	Server    ClientServerSection    `yaml:"server"`    // 服务端地址、TLS、传输心跳/拨号超时
	Tun       ClientTunSection       `yaml:"tun"`       // 本地 TUN 网卡名、MTU、DNS 策略
	Auth      ClientAuthSection      `yaml:"auth"`      // 隧道账号密码
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
// kill_switch 仅由 client.yaml 配置，GUI 登录窗不提供开关；true 时断线阻断 AllowedIPs 出站（须 Windows 管理员）。
type ClientSecuritySection struct {
	KillSwitch bool `yaml:"kill_switch"` // 断线时阻断 AllowedIPs 外出站；默认 false
}

// ClientAuthSection 隧道/Web 登录凭据。
//
// remember_password 为 true 时 GUI 会将 password 明文写回 client.yaml；CLI 也可直接填写 password 免交互。
// ResolveAuth 优先级：yaml auth > 环境变量 HAOVPN_USER / HAOVPN_PASSWORD。
type ClientAuthSection struct {
	Username         string `yaml:"username"`          // 登录名；Validate 要求非空（或 HAOVPN_USER）
	RememberPassword bool   `yaml:"remember_password"` // GUI「记住密码」；true 时 SaveClient 写入 password
	Password         string `yaml:"password,omitempty"` // remember_password=true 时可存明文；false 时 SaveClient 清空
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
// 要求 server.address、auth.username、TLS CA（或非 insecure）。
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
