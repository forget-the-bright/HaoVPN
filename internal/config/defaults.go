package config

// serverYAMLTemplate 服务端默认配置模板（含中文注释，首次启动写入 server.yaml）
const serverYAMLTemplate = `# HaoVPN 服务端配置
# 首次启动自动生成；修改后须重启服务端进程生效。
# 原则：安全 > 简单 > 快。生产现场勿开启 allow_public_bind / sync_password_from_config / allow_plaintext_private_keys。

server:
  # TLS 隧道监听地址（工程师客户端连接此端口；可经 frp 反代）
  # 示例：0.0.0.0:8443（本机所有网卡）、127.0.0.1:8443（仅本机）
  listen: "0.0.0.0:8443"
  tls:
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    # 证书不存在时自动生成约 10 年自签；生产建议 false 并换正式证书
    auto_generate: true

vpn:
  # VPN 虚拟网段（CIDR）；客户端从此池分配 VPN IP
  subnet: "10.88.0.0/24"
  # 服务端 TUN 网关 IP（必须落在 subnet 内，通常为 .1）
  gateway_ip: "10.88.0.1"
  # 隧道内侧 MTU（字节）。默认 1420：比以太网 1500 略小，给 TLS/帧头留余量，减少公网分片。
  # 差链路可试 1280；盲目改为 1500 常导致卡顿。握手会把本值下发给客户端。
  mtu: 1420
  # 传输心跳：间隔与超时（秒）。经 ZeroTier/高丢包链路时建议 timeout≥60～90
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90
  # 发送队列深度（待发帧条数，非字节）。满则丢帧并打 WARN「send queue full」。
  # 默认 256：工控稳妥、延迟可控。两端可不同，互不影响协议。
  # 推荐：512 一般办公；1024 大文件/看电影；2048 极端突发（延迟会升高）。
  # 允许范围 64～8192；越界启动时钳制并打 Warn。≤0 按 256。
  send_queue_size: 256
  # 生产现场应为 true：无 TUN 则拒绝启动（开发冒烟可 false）
  require_tun: true
  # 推送给客户端的 DNS 列表；空则握手回退 gateway_ip
  # 示例：["10.88.0.1", "8.8.8.8"]
  dns_servers: []
  # 同账号第二端策略：
  #   reject_second — 已在线则拒绝（默认，避免双端互踢）
  #   kick_previous — 新连接踢旧会话（两台同时开易互抢）
  session_policy: reject_second
  # 同公网 IP 短窗内重连：顶替旧会话并续算 Rx/Tx（秒）。默认 60；写 -1 关闭
  reconnect_grace_sec: 60

nat:
  # 是否对「允许的工控网段」做 SNAT（客户端访问现场 PLC 等）
  enabled: true
  # 允许经 VPN 访问的局域网 CIDR（并入账号默认 AllowedIPs）
  # 示例：加一条 - "192.168.10.0/24"
  allowed_lan_cidrs:
    - "192.168.1.0/24"
  # Windows ICS 回退时指定出站网卡名；留空则按路由表自动选择
  # outbound_interface: "Ethernet"
  # SNAT 不可用时仅 IP 转发、服务仍启动（health nat_ok=false）；现场交付应 false
  forward_only: false

database:
  # SQLite 主库路径（账号、审计、会话统计等）
  path: "./data/haovpn.db"
  # 账号私钥 AES 密钥：64 字符 hex；留空则首次启动生成 data/.haovpn-key（须备份）
  encryption_key: ""
  # 也可指定密钥文件（与 encryption_key 二选一习惯）
  # encryption_key_file: "./data/.haovpn-key"
  # 审计日志保留天数（超期由 maintenance 清理）
  audit_retention_days: 90
  # 连接事件保留天数
  connection_events_retention_days: 90

api:
  # 管理 API/WebUI 监听主机。默认仅本机；TUN 起来后见 listen_tun
  listen_hosts: ["127.0.0.1"]
  # true（默认）：TUN 就绪后自动追加 VPN 网关 IP，VPN 内用户可访问明文 HTTP 管理口（见 security-hardening）
  # false：仅 bind listen_hosts，降低 VPN 内横向攻击管理口风险
  listen_tun: true
  port: 8080
  # ── 危险选项 · 默认 false ─────────────────────────────────────
  # true 表示已知悉将管理口绑到所有网卡（含公网）的风险并自担后果。
  # listen_hosts 含 0.0.0.0 或 :: 时本项必须为 true，否则拒绝启动。
  allow_public_bind: false
  # ─────────────────────────────────────────────────────────────
  # Web 登录失败锁定：连续失败次数与锁定秒数（Web 与隧道分表，互不影响）
  login_max_attempts: 5
  login_lockout_sec: 900
  # Web Session Cookie 有效期（秒）；28800=8 小时
  session_ttl_sec: 28800
  # 信任的反代源 CIDR；仅 RemoteAddr 命中时才解析 X-Forwarded-For（防锁定绕过）
  # 生产默认留空。内网 nginx 示例：["127.0.0.1/32", "10.0.0.0/8"]
  trusted_proxy_cidrs: []
  # HTTPS 终止或全站 TLS 时设 true，Session Cookie 加 Secure
  secure_cookies: false
  # WebUI 时间展示时区（仅影响控制台页面；SQLite/API JSON 仍为 UTC）
  # 默认 UTC。中国现场常用：Asia/Shanghai 或 GMT+8 或 +08:00
  # 示例：
  #   display_timezone: "UTC"
  #   display_timezone: "Asia/Shanghai"
  #   display_timezone: "GMT+8"
  display_timezone: "UTC"

security:
  # 允许发起 TLS 隧道的客户端源 IP/CIDR；空=不限制
  # 示例：["203.0.113.0/24", "198.51.100.10/32"]
  tunnel_allowed_source_ips: []
  # true 时将 vpn.subnet 并入账号默认 AllowedIPs（分流，非全隧道）
  enforce_split_tunnel: true
  # 仅兼容旧库明文私钥；生产必须 false
  allow_plaintext_private_keys: false
  # true=任意 VPN 账号可互访对方虚拟 IP；细粒度请用控制台托管路由/互访白名单
  allow_all_vpn_peers: false
  # 公网探针防御：识别扫描、落库、可选自动封禁（家里 DDNS 映射建议开启）
  # 封禁表查询始终生效；enabled 只管自动记录与自动封
  probe_defense:
    enabled: true                 # 自动记录/自动封总开关
    record_events: true           # 写入 security_events
    auto_ban: true                # 窗口达阈值写 ip_blocks
    ban_after_events: 8           # 窗口内事件数阈值
    ban_window_sec: 600           # 计数窗口（秒）
    ban_duration_sec: 3600        # 封禁时长秒；0=永久
    event_retention_days: 30      # 事件保留天数
    # 以下特征不计入自动封禁（仍可记事件）
    ignore_signatures_for_ban:
      - connection_reset
      - unexpected_eof
      - auth_failed
    # 封禁豁免：列表内 IP 永不封禁（启动时导入 DB；可在 WebUI 动态维护）
    ban_exempt_ips: []

admin:
  # 库为空时创建的默认 Web 管理员（首次登录通常须改密）
  username: "admin"
  password: "changeme12"
  # 仅 home/开发：用 yaml 密码覆盖库中 admin 并清除须改密。生产务必 false 或不写
  # sync_password_from_config: true

log:
  # 级别：debug / info / warn / error
  level: "info"
  # 滚动文本日志路径（另有 *.live.log 便于实时观测）
  file: "./logs/server.log"
  max_size_mb: 100
  max_backups: 7
  # 结构化历史库（WebUI 工具页检索）；默认 database 同目录 logs.db；-1 关闭
  history_retention_days: 90
  # history_db: "./data/logs.db"
`

// clientYAMLTemplate 客户端默认配置模板
const clientYAMLTemplate = `# HaoVPN 客户端配置
# 首次启动自动生成，或由服务端导出 zip 覆盖。
# vpn_ip / allowed_ips / 私钥均由握手下发，勿在本文件手写 peer 段。

server:
  # 服务端隧道地址 host:port（可走 frp 公网入口）
  # 示例：vpn.example.com:8443
  address: "127.0.0.1:8443"
  tls:
    # 校验服务端证书用的 CA PEM；生产必填
    ca_file: "./certs/ca.crt"
    # true 跳过证书校验（仅开发自签）；生产必须 false
    insecure_skip_verify: false
    # TLS SNI/证书名；空则从 address 主机名推导
    # server_name: "vpn.example.com"
  # 心跳与拨号超时（秒）；损耗链路建议 timeout≥60～90；拨号宜短以便快速重试
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90
  dial_timeout_sec: 3
  # 本端发送队列深度（待发帧条数）。默认 256；上传大文件时可试 1024。
  # 与服务端可不同。范围 64～8192；越界钳制。详见服务端同名注释。
  send_queue_size: 256

tun:
  # 本地虚拟网卡名（Windows Wintun 池名受品牌约束）
  name: "haovpn0"
  # 本地 TUN MTU；握手应答中的 mtu 优先用于实际建口
  mtu: 1420
  # 是否应用握手推送的 DNS；默认 true。不改系统 DNS 可设 dns_from_policy: false
  # dns_from_policy: true

# 账号密码隧道登录（OpenVPN 风格）；也可环境变量 HAOVPN_USER / HAOVPN_PASSWORD
auth:
  username: ""
  # GUI「记住密码」勾选状态；true 时下方 password 会写入本文件
  remember_password: false
  # 仅 remember_password=true 时由 GUI 写入明文；CLI 也可填写以免每次输入
  # 含密码时请限制文件权限（如 0600）且勿提交 git
  # password: ""

# 可选：本机后面的局域网段。非空则登录上报注册表并开启 via 出口（作托管路由出口机）
# 未配置或空列表 = 关闭。勿写入服务端账号 AllowedIPs（那是经网关 NAT 的工控段）
# 示例：
# local_lans:
#   - "192.168.31.0/24"

reconnect:
  # 断线后首次重试等待（秒）与退避上限（秒）
  initial_sec: 1
  max_sec: 3

security:
  # 杀开关（Windows 专用，须管理员；也可在 GUI 托盘「配置」切换，下次连接生效）
  # true：断线/重连期间用 WFP 阻断 allowed_ips 明文出站，防路由清除后泄漏
  # false：断线仅清 TUN 路由（默认，多数现场够用）
  kill_switch: false

# 桌面 GUI 行为（CLI/Windows 服务可忽略本段）
gui:
  # 启动后自动连接：须 remember_password=true 且已保存 password；工控常与 start_minimized 同开
  auto_connect: false
  # 无窗口模式：启动只留托盘，不弹登录/主窗；托盘「显示主窗口」可唤起。关主窗=藏托盘不退出
  start_minimized: false

log:
  level: "info"
  file: "./logs/client.log"
  max_size_mb: 50
  max_backups: 3
`
