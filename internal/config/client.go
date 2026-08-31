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
//   GUI — 桌面客户端行为（自动连接、无窗口托盘）；
//   LocalLANs — 可选本地网段列表；非空则登录上报并开启 via 出口；
//   Security — Kill-switch 等客户端安全选项；
//   Windows — 仅 Windows 生效的网卡加速开关（IP Helper）；其它平台忽略；
//   Reconnect — 断线指数退避；
//   Log — 日志级别与滚动文件路径。
//
// vpn_ip / allowed_ips / gateway / 私钥均由握手下发，client.yaml 不含 peer 段。
// ApplyDefaults 填充缺省；Validate 在连接前校验。
type ClientConfig struct {
	Server    ClientServerSection    `yaml:"server"`     // 服务端地址、TLS、传输心跳/拨号超时
	Tun       ClientTunSection       `yaml:"tun"`        // 本地 TUN 网卡名、MTU、DNS 策略
	Auth      ClientAuthSection      `yaml:"auth"`       // 隧道账号密码
	GUI       ClientGUISection       `yaml:"gui"`        // 桌面 GUI 行为（自动连接、无窗口）
	LocalLANs []string               `yaml:"local_lans"` // 手动配置的本地网段；空=关闭 via 广告与出口
	Security  ClientSecuritySection  `yaml:"security"`   // Kill-switch 等客户端安全选项
	Windows   ClientWindowsSection   `yaml:"windows"`    // Windows 专用：IP Helper（其它 OS 忽略）
	Reconnect ReconnectSection       `yaml:"reconnect"`  // 断线指数退避重连
	Log       LogSection             `yaml:"log"`        // 日志级别与文件路径
}

// ClientWindowsSection 仅 Windows 生效的网卡/子进程加速选项。
//
// 非 Windows 平台读入后忽略，不报错。默认 use_ip_helper=true。
// 旧 yaml 若仍含 ps_resident 键：标准 Unmarshal 忽略未知字段，无影响。
type ClientWindowsSection struct {
	// UseIPHelper 地址探测/ICS/配 IP/路由/DNS 优先 IP Helper；失败回退 netsh/route/PS。nil=默认 true。
	UseIPHelper *bool `yaml:"use_ip_helper"`
}

// UseIPHelperEnabled 是否启用 IP Helper 优先路径（未配置时默认 true）。
func (w ClientWindowsSection) UseIPHelperEnabled() bool {
	if w.UseIPHelper == nil {
		return true
	}
	return *w.UseIPHelper
}

// ClientGUISection 桌面客户端（Fyne）行为选项；CLI/服务模式可忽略。
type ClientGUISection struct {
	// AutoConnect 启动 GUI 后自动拨号；须 remember_password 且 password 非空。
	AutoConnect bool `yaml:"auto_connect"`
	// StartMinimized 无窗口模式：启动仅托盘，可再「显示主窗口」唤起。
	StartMinimized bool `yaml:"start_minimized"`
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
	SendQueueSize        int              `yaml:"send_queue_size"`        // 传输待发帧队列深度；默认 256；范围 64～8192
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
	c.Server.SendQueueSize = clampSendQueueLogged("server.send_queue_size", c.Server.SendQueueSize)
	if strings.TrimSpace(c.Log.Level) == "" {
		c.Log.Level = "info"
	}
	if strings.TrimSpace(c.Log.File) == "" {
		c.Log.File = "./logs/client.log"
	}
}

// CanAutoConnect 是否允许 GUI 启动后自动拨号（须记住密码且密码非空）。
func (c *ClientConfig) CanAutoConnect() bool {
	if c == nil || !c.GUI.AutoConnect {
		return false
	}
	if !c.Auth.RememberPassword {
		return false
	}
	_, pass := c.ResolveAuth()
	return strings.TrimSpace(pass) != ""
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
	if c.GUI.AutoConnect && !c.CanAutoConnect() {
		return fmt.Errorf("配置错误: gui.auto_connect 须同时开启 auth.remember_password 并保存密码")
	}
	if err := ValidateTunName(c.Tun.Name); err != nil {
		return fmt.Errorf("配置错误: %w", err)
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
