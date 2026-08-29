package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"haovpn/internal/netutil"
)

// ServerConfig 服务端完整配置（对应 server.yaml）。
//
// 各 YAML 段职责：
//   Server — TLS 隧道监听与证书；
//   VPN — 虚拟网段、网关、DNS 推送与心跳；
//   NAT — 工控网段 SNAT/转发（netstack）；
//   Database — SQLite 路径、字段加密与审计保留；
//   API — 管理 Web/API HTTP 监听与安全；
//   Security — 隧道源 IP 限制与分流策略；
//   Admin — 首次 bootstrap 管理员；
//   Log — 滚动文件日志与 logs.db 结构化历史。
//
// ApplyDefaults 在 Validate 前填充缺省；握手不下发本结构。
type ServerConfig struct {
	Server   ServerSection   `yaml:"server"`   // TLS 隧道监听
	VPN      VPNSection      `yaml:"vpn"`      // 虚拟网段、网关、DNS 推送
	NAT      NATSection      `yaml:"nat"`      // 工控网段 SNAT/转发
	Database DatabaseSection `yaml:"database"` // SQLite 与字段加密
	API      APISection      `yaml:"api"`      // 管理 Web/API 监听
	Security SecuritySection `yaml:"security"` // 隧道源 IP 与分流策略
	Admin    AdminSection    `yaml:"admin"`    // 首次 bootstrap 管理员
	Log      LogSection      `yaml:"log"`      // 文件日志与 logs.db 历史
}

// ServerSection 隧道 TLS 监听地址与证书配置。
//
// listen 须为 host:port；tls 段支持 PEM 路径或 auto_generate 自签。
type ServerSection struct {
	Listen string     `yaml:"listen"` // host:port；Validate 必填
	TLS    TLSSection `yaml:"tls"`    // 证书路径或自签生成
}

// TLSSection TLS 证书路径与自签证书生成选项。
//
// auto_generate 为 true 且无 cert/key 文件时启动流程自动生成；cert_sans 补充 SAN。
type TLSSection struct {
	CertFile     string   `yaml:"cert_file"`     // 服务端证书 PEM；与 key_file 成对
	KeyFile      string   `yaml:"key_file"`      // 服务端私钥 PEM
	AutoGenerate bool     `yaml:"auto_generate"` // true 且无证书时自动生成自签
	CertSANs     []string `yaml:"cert_sans"`     // 自签证书额外 SAN（主机名或 IP）
}

// 会话策略常量：同账号第二端连接时的行为（vpn.session_policy）。
const (
	// SessionPolicyRejectSecond 已有在线会话则拒绝新连接（默认，避免双端互踢重连循环）。
	SessionPolicyRejectSecond = "reject_second"
	// SessionPolicyKickPrevious 新连接踢掉旧会话（旧行为；两台同时开会互相抢线）。
	SessionPolicyKickPrevious = "kick_previous"
)

// VPNSection VPN 虚拟网段、网关、MTU 与推送给客户端的运行时策略。
//
// dns_servers 经握手下发；空则客户端回退 gateway_ip；require_tun 为 true 时 TUN 失败则拒绝启动。
type VPNSection struct {
	Subnet               string   `yaml:"subnet"`                 // CIDR；Validate 必填
	GatewayIP            string   `yaml:"gateway_ip"`             // 子网内网关；Validate 必填且在子网内
	MTU                  int      `yaml:"mtu"`                    // 隧道 MTU；默认 netutil.DefaultMTU
	HeartbeatIntervalSec int      `yaml:"heartbeat_interval_sec"` // 传输心跳间隔；默认 netutil 常量
	HeartbeatTimeoutSec  int      `yaml:"heartbeat_timeout_sec"`  // 心跳超时；默认 netutil 常量
	RequireTun           bool     `yaml:"require_tun"`            // true 时 TUN 创建失败则拒绝启动
	DNSServers           []string `yaml:"dns_servers"`            // 握手推送给客户端；可空则回退 gateway
	SessionPolicy        string   `yaml:"session_policy"`         // reject_second（默认）| kick_previous
}

// NATSection 工控/局域网 SNAT 与 IP 转发配置（internal/netstack）。
//
// allowed_lan_cidrs 非空时并入账号默认 AllowedIPs；forward_only 允许 SNAT 失败时服务仍启动。
type NATSection struct {
	Enabled           bool     `yaml:"enabled"`            // 是否启用 SNAT
	AllowedLANCIDRs   []string `yaml:"allowed_lan_cidrs"`  // 允许经 VPN 访问的 LAN CIDR；并入默认 AllowedIPs
	OutboundInterface string   `yaml:"outbound_interface"` // Windows ICS 公网侧网卡名；可选
	ForwardOnly       bool     `yaml:"forward_only"`       // SNAT 不可用时仅转发、不 Fatal（nat_ok=false）
}

// DatabaseSection SQLite 数据库路径、私钥加密与审计/连接事件保留策略。
//
// encryption_key 为 64 字符 hex；缺省可读 encryption_key_file（默认 data/.haovpn-key）。
type DatabaseSection struct {
	Path                          string `yaml:"path"`                            // 数据库文件；Validate 必填
	EncryptionKey                 string `yaml:"encryption_key"`                  // 64 字符 hex（32 字节 AES-256）；可选
	EncryptionKeyFile             string `yaml:"encryption_key_file"`             // 密钥文件；默认 data/.haovpn-key
	AuditRetentionDays            int    `yaml:"audit_retention_days"`            // 审计日志保留天；默认 DefaultRetentionDays
	ConnectionEventsRetentionDays int    `yaml:"connection_events_retention_days"` // 连接事件保留天；默认同上
}

// APISection 管理 API / WebUI HTTP 监听与安全参数。
//
// listen_hosts 含 0.0.0.0/:: 时须 allow_public_bind: true；session_ttl_sec 控制 Cookie 有效期。
type APISection struct {
	ListenHosts      []string `yaml:"listen_hosts"`       // 绑定地址列表；默认 127.0.0.1
	Port             int      `yaml:"port"`               // 端口；Validate 须 1–65535
	AllowPublicBind  bool     `yaml:"allow_public_bind"`  // 含 0.0.0.0/:: 时须显式 true
	LoginMaxAttempts     int      `yaml:"login_max_attempts"`      // 登录失败上限；0 用内置默认
	LoginLockoutSec      int      `yaml:"login_lockout_sec"`       // 锁定时长秒
	SessionTTLSec        int      `yaml:"session_ttl_sec"`         // Web 会话 Cookie 有效期秒
	TrustedProxyCIDRs    []string `yaml:"trusted_proxy_cidrs"`   // 信任反代 CIDR；空则 clientIP 仅用 RemoteAddr（防 XFF 绕过锁定）
	SecureCookies        bool     `yaml:"secure_cookies"`          // true 或 HTTPS 请求时 Session Cookie 设 Secure
}

// SecuritySection 隧道接入源 IP 限制、分流策略与公网探针防御。
//
// enforce_split_tunnel 为 true 时将 vpn.subnet 并入账号默认 AllowedIPs。
type SecuritySection struct {
	TunnelAllowedSourceIPs []string            `yaml:"tunnel_allowed_source_ips"` // 允许发起 TLS 隧道的源 CIDR；空不限制
	EnforceSplitTunnel     bool                `yaml:"enforce_split_tunnel"`      // true 时将 VPN.Subnet 并入账号默认 AllowedIPs
	ProbeDefense           ProbeDefenseSection `yaml:"probe_defense"`             // 公网扫描识别、落库与自动封禁
}

// ProbeDefenseSection 公网对隧道口的探针防御（security.probe_defense）。
//
// Enabled / RecordEvents / AutoBan 使用 *bool：YAML 未写为 nil，ApplyDefaults 填默认 true；
// 显式 false 永不被改回（避免 bool 零值与「未配置」混淆）。
type ProbeDefenseSection struct {
	Enabled                *bool    `yaml:"enabled"`                  // 自动记录/自动封总开关；nil→默认 true；封禁表查询不依赖本项
	RecordEvents           *bool    `yaml:"record_events"`            // 是否写入 security_events；nil→默认 true
	AutoBan                *bool    `yaml:"auto_ban"`                 // 窗口达阈值是否自动写 ip_blocks；nil→默认 true
	BanAfterEvents         int      `yaml:"ban_after_events"`         // 窗口内事件数阈值；≤0 默认 8
	BanWindowSec           int      `yaml:"ban_window_sec"`           // 计数窗口秒；≤0 默认 600
	BanDurationSec         int      `yaml:"ban_duration_sec"`         // 封禁时长秒；0=永久；整段未配置时默认 3600
	EventRetentionDays     int      `yaml:"event_retention_days"`     // 事件保留天；≤0 默认 30
	IgnoreSignaturesForBan []string `yaml:"ignore_signatures_for_ban"` // 不计入自动封禁的特征码；nil 填默认列表
}

// BoolOr 解引用 *bool；nil 时返回 def（供 ApplyDefaults 前后读取）。
func BoolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// BoolPtr 返回 bool 指针（写测试与 ApplyDefaults 填充用）。
func BoolPtr(v bool) *bool { return &v }

// IsEnabled 探针自动防御是否开启（ApplyDefaults 后 Enabled 非 nil）。
func (p ProbeDefenseSection) IsEnabled() bool { return BoolOr(p.Enabled, true) }

// IsRecordEvents 是否落库安全事件。
func (p ProbeDefenseSection) IsRecordEvents() bool { return BoolOr(p.RecordEvents, true) }

// IsAutoBan 是否自动封禁。
func (p ProbeDefenseSection) IsAutoBan() bool { return BoolOr(p.AutoBan, true) }

// AdminSection 首次启动（users 表为空）时创建的默认 Web 管理员账号。
//
// sync_password_from_config 为 true 时用 yaml 密码覆盖库中 admin（开发/home 环境）。
type AdminSection struct {
	Username               string `yaml:"username"`                  // 管理员登录名；Validate 必填
	Password               string `yaml:"password"`                  // 初始密码；生产环境应首次登录改密
	SyncPasswordFromConfig bool   `yaml:"sync_password_from_config"` // true 时用 yaml 密码覆盖库中 admin（开发/home）
}

// LogSection 滚动文件日志与可选结构化 history 库（logs.db）。
//
// history_retention_days：0 经 ApplyDefaults 为 90；-1 关闭 logs.db 写入。
type LogSection struct {
	Level                string `yaml:"level"`                  // debug/info/warn/error；ApplyDefaults 默认 info
	File                 string `yaml:"file"`                   // 滚动日志路径
	MaxSizeMB            int    `yaml:"max_size_mb"`            // 单文件大小上限 MB
	MaxBackups           int    `yaml:"max_backups"`            // 保留备份文件数
	HistoryDB            string `yaml:"history_db"`             // 结构化历史库路径；空则 database.path 同目录 logs.db
	HistoryRetentionDays int    `yaml:"history_retention_days"` // 历史库保留天；0 经 ApplyDefaults 为 90；-1 关闭
}

// ApplyDefaults 填充服务端 YAML 缺省项。
//
// 与 client.ApplyDefaults 对称；Validate 前必须调用。
func (c *ServerConfig) ApplyDefaults() {
	if c.VPN.MTU <= 0 {
		c.VPN.MTU = netutil.DefaultMTU
	}
	if c.VPN.HeartbeatIntervalSec <= 0 {
		c.VPN.HeartbeatIntervalSec = netutil.DefaultHeartbeatIntervalSec
	}
	if c.VPN.HeartbeatTimeoutSec <= 0 {
		c.VPN.HeartbeatTimeoutSec = netutil.DefaultHeartbeatTimeoutSec
	}
	if strings.TrimSpace(c.VPN.SessionPolicy) == "" {
		c.VPN.SessionPolicy = SessionPolicyRejectSecond
	}
	if len(c.API.ListenHosts) == 0 {
		c.API.ListenHosts = []string{"127.0.0.1"}
	}
	if c.Database.AuditRetentionDays <= 0 {
		c.Database.AuditRetentionDays = DefaultRetentionDays
	}
	if c.Database.ConnectionEventsRetentionDays <= 0 {
		c.Database.ConnectionEventsRetentionDays = DefaultRetentionDays
	}
	// 旧 yaml 缺 history_retention_days 时为 0，默认启用 90 天；设为 -1 可关闭结构化历史库
	if c.Log.HistoryRetentionDays == 0 {
		c.Log.HistoryRetentionDays = DefaultRetentionDays
	}
	c.applyProbeDefenseDefaults()
}

// applyProbeDefenseDefaults 填充探针防御缺省。
//
// *bool 为 nil 表示「未写」→ 填 true；显式 false 保持 false。
// BanDurationSec：仅当整段布尔均未配置且数值全为 0 时视为首次启用，填 3600；
// 用户显式写 ban_duration_sec: 0 表示永久封禁（与布尔已配置可区分）。
func (c *ServerConfig) applyProbeDefenseDefaults() {
	pd := &c.Security.ProbeDefense
	freshSection := pd.Enabled == nil && pd.RecordEvents == nil && pd.AutoBan == nil &&
		pd.BanAfterEvents == 0 && pd.BanWindowSec == 0 && pd.BanDurationSec == 0 &&
		pd.EventRetentionDays == 0 && pd.IgnoreSignaturesForBan == nil
	if pd.Enabled == nil {
		pd.Enabled = BoolPtr(true)
	}
	if pd.RecordEvents == nil {
		pd.RecordEvents = BoolPtr(true)
	}
	if pd.AutoBan == nil {
		pd.AutoBan = BoolPtr(true)
	}
	if freshSection {
		pd.BanDurationSec = 3600
	}
	if pd.BanAfterEvents <= 0 {
		pd.BanAfterEvents = 8
	}
	if pd.BanWindowSec <= 0 {
		pd.BanWindowSec = 600
	}
	if pd.EventRetentionDays <= 0 {
		pd.EventRetentionDays = 30
	}
	if pd.IgnoreSignaturesForBan == nil {
		pd.IgnoreSignaturesForBan = []string{"connection_reset", "unexpected_eof", "auth_failed"}
	}
}

// Validate 校验服务端配置必填项、取值范围与安全策略。
//
// 校验 vpn 子网/网关、api 端口、公网绑定策略、admin 用户名及 NAT/Security CIDR 列表。
// 返回：字段名含中文说明的 error。
func (c *ServerConfig) Validate() error {
	c.ApplyDefaults()
	if strings.TrimSpace(c.Server.Listen) == "" {
		return fmt.Errorf("配置错误: server.listen 不能为空")
	}
	if strings.TrimSpace(c.VPN.Subnet) == "" {
		return fmt.Errorf("配置错误: vpn.subnet 不能为空")
	}
	if strings.TrimSpace(c.VPN.GatewayIP) == "" {
		return fmt.Errorf("配置错误: vpn.gateway_ip 不能为空")
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		return fmt.Errorf("配置错误: database.path 不能为空")
	}
	if c.API.Port <= 0 || c.API.Port > 65535 {
		return fmt.Errorf("配置错误: api.port 须在 1-65535 之间")
	}
	// 防误配：0.0.0.0/:: 须显式 allow_public_bind
	if netutil.HasWildcardListenHost(c.API.ListenHosts) && !c.API.AllowPublicBind {
		return fmt.Errorf("配置错误: api.listen_hosts 含 0.0.0.0/:: 但 allow_public_bind 为 false；若确需公网监听请设 allow_public_bind: true")
	}
	if strings.TrimSpace(c.Admin.Username) == "" {
		return fmt.Errorf("配置错误: admin.username 不能为空")
	}
	switch strings.TrimSpace(c.VPN.SessionPolicy) {
	case SessionPolicyRejectSecond, SessionPolicyKickPrevious:
	default:
		return fmt.Errorf("配置错误: vpn.session_policy 须为 %s 或 %s", SessionPolicyRejectSecond, SessionPolicyKickPrevious)
	}
	if err := netutil.ValidateSubnetGateway(c.VPN.Subnet, c.VPN.GatewayIP); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if err := netutil.ValidateCIDRList("nat.allowed_lan_cidrs", c.NAT.AllowedLANCIDRs); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if err := netutil.ValidateCIDRList("security.tunnel_allowed_source_ips", c.Security.TunnelAllowedSourceIPs); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if err := netutil.ValidateCIDRList("api.trusted_proxy_cidrs", c.API.TrustedProxyCIDRs); err != nil {
		return fmt.Errorf("配置错误: %w", err)
	}
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		if net.ParseIP(strings.TrimSpace(c.Server.Listen)) == nil {
			return fmt.Errorf("配置错误: server.listen 须为 host:port 格式: %w", err)
		}
	}
	return nil
}

// HistoryLogEnabled 是否启用 logs.db 结构化历史写入。
//
// 返回：history_retention_days > 0 时为 true。
func (c *ServerConfig) HistoryLogEnabled() bool {
	return c.Log.HistoryRetentionDays > 0
}

// ResolveHistoryDBPath 返回结构化日志库 logs.db 的路径。
//
// 优先 log.history_db；空则 database.path 同目录下的 logs.db。
func (c *ServerConfig) ResolveHistoryDBPath() string {
	if p := strings.TrimSpace(c.Log.HistoryDB); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(c.Database.Path), "logs.db")
}
