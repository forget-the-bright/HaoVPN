package config

// serverYAMLTemplate 服务端默认配置模板（含中文注释，首次启动写入 server.yaml）
const serverYAMLTemplate = `# HaoVPN 服务端配置
# 首次启动自动生成，修改后重启生效

server:
  # TLS 隧道监听地址（工程师客户端连接此端口，可走 frp）
  listen: "0.0.0.0:8443"
  tls:
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    # 证书不存在时自动生成 10 年自签（生产建议关闭并换正式证书）
    auto_generate: true

vpn:
  # VPN 虚拟网段，客户端从此池分配 IP
  subnet: "10.88.0.0/24"
  # 服务端 TUN 网关 IP（须在 subnet 内）
  gateway_ip: "10.88.0.1"
  mtu: 1420
  # 心跳：损耗链路（ZeroTier 等）建议 timeout≥60～90，避免短暂抖动整段断线
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90
  # 生产现场应为 true：无 TUN 则拒绝启动（开发冒烟可设 false）
  require_tun: true
  # 推送给客户端的 DNS（可空；握手时未配置则回退 gateway_ip）
  dns_servers: []

nat:
  # 允许 VPN 客户端访问的现场工控网段（SNAT 放行范围）
  enabled: true
  allowed_lan_cidrs:
    - "192.168.1.0/24"
  # Windows ICS 回退：出站网卡名（ZeroTier 等；留空则按路由表自动选择）
  # outbound_interface: "ZeroTier One [xxxxxxxx]"
  # SNAT 不可用时仅 IP 转发、服务仍启动（health nat_ok=false；现场交付应设为 false）
  forward_only: false

database:
  path: "./data/haovpn.db"
  # 数据库内 peer 私钥 AES 加密密钥：留空则首次启动生成 data/.haovpn-key（须备份）
  encryption_key: ""
  # encryption_key_file: "./data/.haovpn-key"
  # 审计/连接事件保留天数（超期自动清理）
  audit_retention_days: 90
  connection_events_retention_days: 90

api:
  # 管理 API/WebUI 监听主机。默认仅本机；TUN 启动后会自动追加 VPN 网卡 IP
  listen_hosts: ["127.0.0.1"]
  port: 8080
  # ── 危险选项 · 默认 false ─────────────────────────────────────
  # 设为 true 表示：您已知悉将管理口绑定到所有网卡（含公网）的风险，并自行承担后果，与软件无关。
  # 仅建议在开发联调、内网测试时使用；生产现场务必保持 false。
  # 当 listen_hosts 含 0.0.0.0 或 :: 时，本项必须为 true，否则拒绝启动。
  allow_public_bind: false
  # ─────────────────────────────────────────────────────────────
  login_max_attempts: 5
  login_lockout_sec: 900
  session_ttl_sec: 28800

security:
  tunnel_allowed_source_ips: []
  enforce_split_tunnel: true

admin:
  username: "admin"
  password: "changeme"
  # sync_password_from_config: true  # 仅 home 开发，生产务必 false

log:
  level: "info"
  file: "./logs/server.log"
  max_size_mb: 100
  max_backups: 7
  # 结构化历史日志库（WebUI 分页检索）；同目录 logs.db；-1 关闭
  history_retention_days: 90
  # history_db: "./data/logs.db"
`

// clientYAMLTemplate 客户端默认配置模板
const clientYAMLTemplate = `# HaoVPN 客户端配置
# 首次启动自动生成，或由服务端导出 zip 覆盖

server:
  address: "127.0.0.1:8443"
  tls:
    ca_file: "./certs/ca.crt"
    insecure_skip_verify: false
  # 与服务端对齐；损耗链路建议 timeout≥60～90；拨号超时宜短以便快速重试
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90
  dial_timeout_sec: 3

tun:
  name: "haovpn0"
  mtu: 1420

# 账号密码隧道登录（OpenVPN 风格）；也可环境变量 HAOVPN_USER / HAOVPN_PASSWORD
auth:
  username: ""
  # remember_password：GUI「记住密码」勾选状态；true 时下方 password 会写入本文件
  remember_password: false
  # password：仅 remember_password=true 时由 GUI 写入明文；CLI 也可填写以免每次输入
  # 注意：含密码时本文件为敏感信息，请限制权限（0600）且勿提交 git
  # password: ""

reconnect:
  initial_sec: 1
  max_sec: 3

security:
  # 杀开关 kill_switch（Windows 专用，须管理员；GUI 登录窗不提供开关，仅改本文件）
  # true：VPN 断线/重连期间，用 WFP 阻断 allowed_ips 工控网段明文出站，防路由清除后泄漏
  # false：断线仅清 TUN 路由，不额外阻断（默认，多数现场够用）
  kill_switch: false

log:
  level: "info"
  file: "./logs/client.log"
  max_size_mb: 50
  max_backups: 3
`
