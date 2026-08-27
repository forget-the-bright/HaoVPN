package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// ServerConfig 服务端完整配置结构（对应 server.yaml）
type ServerConfig struct {
	Server   ServerSection   `yaml:"server"`
	VPN      VPNSection      `yaml:"vpn"`
	NAT      NATSection      `yaml:"nat"`
	Database DatabaseSection `yaml:"database"`
	API      APISection      `yaml:"api"`
	Security SecuritySection `yaml:"security"`
	Admin    AdminSection    `yaml:"admin"`
	Log      LogSection      `yaml:"log"`
}

// ServerSection 隧道 TLS 监听配置
type ServerSection struct {
	Listen string    `yaml:"listen"`
	TLS    TLSSection `yaml:"tls"`
}

// TLSSection TLS 证书路径与自签开关
type TLSSection struct {
	CertFile     string   `yaml:"cert_file"`
	KeyFile      string   `yaml:"key_file"`
	AutoGenerate bool     `yaml:"auto_generate"`
	CertSANs     []string `yaml:"cert_sans"` // 自签证书额外 SAN（主机名或 IP）
}

// VPNSection VPN 虚拟网段与心跳
type VPNSection struct {
	Subnet               string `yaml:"subnet"`
	GatewayIP            string `yaml:"gateway_ip"`
	MTU                  int    `yaml:"mtu"`
	HeartbeatIntervalSec int    `yaml:"heartbeat_interval_sec"`
	HeartbeatTimeoutSec  int    `yaml:"heartbeat_timeout_sec"`
	RequireTun           bool     `yaml:"require_tun"` // true 时 TUN 创建失败则拒绝启动
	DNSServers           []string `yaml:"dns_servers"` // 推送给客户端的 DNS（可空，握手时可回退 gateway）
}

// NATSection 工控网段 SNAT 配置
type NATSection struct {
	Enabled           bool     `yaml:"enabled"`
	AllowedLANCIDRs   []string `yaml:"allowed_lan_cidrs"`
	OutboundInterface string   `yaml:"outbound_interface"` // 可选：Windows ICS 公网侧网卡名
	ForwardOnly       bool     `yaml:"forward_only"`       // SNAT 不可用时仅 IP 转发、不 Fatal（nat_ok=false）
}

// DatabaseSection SQLite 路径与字段加密密钥
type DatabaseSection struct {
	Path                        string `yaml:"path"`
	EncryptionKey               string `yaml:"encryption_key"`      // 可选：64 字符 hex（32 字节 AES-256）
	EncryptionKeyFile           string `yaml:"encryption_key_file"` // 可选：默认 data/.haovpn-key
	AuditRetentionDays          int    `yaml:"audit_retention_days"`
	ConnectionEventsRetentionDays int `yaml:"connection_events_retention_days"`
}

// APISection 管理 API / WebUI 配置
type APISection struct {
	ListenHosts       []string `yaml:"listen_hosts"`
	Port              int      `yaml:"port"`
	AllowPublicBind   bool     `yaml:"allow_public_bind"`
	LoginMaxAttempts  int      `yaml:"login_max_attempts"`
	LoginLockoutSec   int      `yaml:"login_lockout_sec"`
	SessionTTLSec     int      `yaml:"session_ttl_sec"`
}

// SecuritySection 隧道准入与安全策略
type SecuritySection struct {
	TunnelAllowedSourceIPs []string `yaml:"tunnel_allowed_source_ips"`
	EnforceSplitTunnel     bool     `yaml:"enforce_split_tunnel"`
}

// AdminSection 首次初始化默认管理员（仅 users 表为空时生效）
type AdminSection struct {
	Username                string `yaml:"username"`
	Password                string `yaml:"password"`
	SyncPasswordFromConfig  bool   `yaml:"sync_password_from_config"` // true 时用 yaml 密码覆盖库中 admin（开发/home）
}

// LogSection 日志配置
type LogSection struct {
	Level                string `yaml:"level"`
	File                 string `yaml:"file"`
	MaxSizeMB            int    `yaml:"max_size_mb"`
	MaxBackups           int    `yaml:"max_backups"`
	HistoryDB            string `yaml:"history_db"`             // 留空则与 database.path 同目录 logs.db
	HistoryRetentionDays int    `yaml:"history_retention_days"` // 0=关闭结构化历史库
}

// Validate 校验服务端配置必填项与取值范围，错误信息指明字段名。
func (c *ServerConfig) Validate() error {
	if strings.TrimSpace(c.Server.Listen) == "" {
		return fmt.Errorf("配置错误: server.listen 不能为空")
	}
	if strings.TrimSpace(c.VPN.Subnet) == "" {
		return fmt.Errorf("配置错误: vpn.subnet 不能为空")
	}
	if strings.TrimSpace(c.VPN.GatewayIP) == "" {
		return fmt.Errorf("配置错误: vpn.gateway_ip 不能为空")
	}
	if c.VPN.MTU <= 0 {
		c.VPN.MTU = 1420
	}
	if c.VPN.HeartbeatIntervalSec <= 0 {
		c.VPN.HeartbeatIntervalSec = 15
	}
	if c.VPN.HeartbeatTimeoutSec <= 0 {
		c.VPN.HeartbeatTimeoutSec = 90
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		return fmt.Errorf("配置错误: database.path 不能为空")
	}
	if c.API.Port <= 0 || c.API.Port > 65535 {
		return fmt.Errorf("配置错误: api.port 须在 1-65535 之间")
	}
	if len(c.API.ListenHosts) == 0 {
		c.API.ListenHosts = []string{"127.0.0.1"}
	}
	// 防误配：0.0.0.0/:: 须显式 allow_public_bind
	if containsWildcardHost(c.API.ListenHosts) && !c.API.AllowPublicBind {
		return fmt.Errorf("配置错误: api.listen_hosts 含 0.0.0.0/:: 但 allow_public_bind 为 false；若确需公网监听请设 allow_public_bind: true")
	}
	if strings.TrimSpace(c.Admin.Username) == "" {
		return fmt.Errorf("配置错误: admin.username 不能为空")
	}
	if err := validateSubnetGateway(c.VPN.Subnet, c.VPN.GatewayIP); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if err := validateCIDRList("nat.allowed_lan_cidrs", c.NAT.AllowedLANCIDRs); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if err := validateCIDRList("security.tunnel_allowed_source_ips", c.Security.TunnelAllowedSourceIPs); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		if net.ParseIP(strings.TrimSpace(c.Server.Listen)) == nil {
			return fmt.Errorf("配置错误: server.listen 须为 host:port 格式: %w", err)
		}
	}
	c.applyDefaults()
	return nil
}

// applyDefaults 填充 retention 等默认值（旧 yaml 缺字段时兼容）。
func (c *ServerConfig) applyDefaults() {
	if c.Database.AuditRetentionDays <= 0 {
		c.Database.AuditRetentionDays = 90
	}
	if c.Database.ConnectionEventsRetentionDays <= 0 {
		c.Database.ConnectionEventsRetentionDays = 90
	}
	// 旧 yaml 缺 history_retention_days 时为 0，默认启用 90 天；设为 -1 可关闭结构化历史库
	if c.Log.HistoryRetentionDays == 0 {
		c.Log.HistoryRetentionDays = 90
	}
}

// HistoryLogEnabled 是否写入 logs.db（history_retention_days > 0）。
func (c *ServerConfig) HistoryLogEnabled() bool {
	return c.Log.HistoryRetentionDays > 0
}

// ResolveHistoryDBPath 返回结构化日志库路径。
func (c *ServerConfig) ResolveHistoryDBPath() string {
	if p := strings.TrimSpace(c.Log.HistoryDB); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(c.Database.Path), "logs.db")
}
