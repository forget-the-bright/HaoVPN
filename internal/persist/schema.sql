-- HaoVPN SQLite 数据库 schema
-- VPN 账号合一：users 表同时承载 Web 登录与隧道身份（无 peers 表）
-- 启动时由 persist.Open 执行本文件（CREATE TABLE IF NOT EXISTS）；无运行时 v1→v3 迁移

-- VPN 账号（Web 登录 + 隧道身份）
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    is_admin INTEGER NOT NULL DEFAULT 0,
    -- 隧道身份（admin 等纯管理账号可为空 public_key）
    public_key TEXT UNIQUE,
    private_key_enc TEXT,
    vpn_ip TEXT,
    allowed_ips TEXT NOT NULL DEFAULT '[]',
    ip_mode TEXT NOT NULL DEFAULT 'fixed',
    -- 默认租约秒数与 persist.DefaultIPLeaseSec（86400）同源
    ip_lease_sec INTEGER NOT NULL DEFAULT 86400,
    policy_ver INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- IP 地址池占用记录
CREATE TABLE IF NOT EXISTS ip_allocations (
    ip TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    allocated_at TEXT NOT NULL DEFAULT (datetime('now')),
    released_at TEXT,
    lease_until TEXT
);

-- 连接上下线/断线事件
CREATE TABLE IF NOT EXISTS connection_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    event_type TEXT NOT NULL,
    remote_addr TEXT,
    detail_json TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_connection_events_user_time ON connection_events(user_id, created_at);

-- 当前会话与累计流量统计
CREATE TABLE IF NOT EXISTS session_stats (
    user_id INTEGER PRIMARY KEY REFERENCES users(id),
    connected_at TEXT,
    last_heartbeat TEXT,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    reconnect_count INTEGER NOT NULL DEFAULT 0,
    remote_addr TEXT
);

-- 管理操作审计日志
CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id INTEGER REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT,
    target_id INTEGER,
    client_ip TEXT,
    detail_json TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at);

-- 公网探针 / TLS 拒绝 / 非法帧 / 握手拒绝等安全事件流水（无 user_id，与 audit_logs 分离）
-- phase / signature / action 存英文码；中文含义见 probedefense.labels 与 docs/security-hardening.md
CREATE TABLE IF NOT EXISTS security_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    client_ip TEXT NOT NULL,          -- 源 IP
    client_port TEXT,                 -- 源端口（可空）
    phase TEXT NOT NULL,              -- 阶段：tcp_accept / tls / frame / handshake / ban_hit
    signature TEXT NOT NULL,          -- 特征：http_get / auth_failed / account_online / ...
    action TEXT NOT NULL,             -- 动作：rejected / banned_hit / auto_banned / manual_banned
    detail_json TEXT,                 -- 附加 JSON（如原始错误摘要）
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_security_events_created ON security_events(created_at);
CREATE INDEX IF NOT EXISTS idx_security_events_ip_time ON security_events(client_ip, created_at);

-- IP 封禁状态（自动或手动）；同 IP 一行，续封更新 expires_at
-- Enabled=0 表示已解封保留痕迹；查询生效封禁看 enabled=1 且未过期
CREATE TABLE IF NOT EXISTS ip_blocks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ip TEXT NOT NULL UNIQUE,          -- 被封 IP
    reason TEXT NOT NULL DEFAULT '',  -- 原因文案
    source TEXT NOT NULL DEFAULT 'auto',  -- auto=自动 / manual=管理员
    signature TEXT,                   -- 触发自动封时的特征码
    hits INTEGER NOT NULL DEFAULT 0,  -- 封禁后再连命中次数
    expires_at TEXT,                  -- NULL=永久（直至手动解封）
    enabled INTEGER NOT NULL DEFAULT 1, -- 1=生效 0=已解封
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_hit_at TEXT                  -- 最近一次撞封时间
);
CREATE INDEX IF NOT EXISTS idx_ip_blocks_enabled ON ip_blocks(enabled);

-- 封禁豁免：列表内 IP/CIDR 永不自动/手动封禁，且不受 ip_blocks 生效记录影响
-- enabled=0 表示已移除；source=yaml_import 为 server.yaml 启动导入
CREATE TABLE IF NOT EXISTS ip_ban_exempt (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ip TEXT NOT NULL UNIQUE,
    note TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'manual',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_ip_ban_exempt_enabled ON ip_ban_exempt(enabled);

-- VPN 账号互访白名单：user_id 可访问 peer_user_id 的当前 vpn_ip/32（默认无行=禁止互访）
CREATE TABLE IF NOT EXISTS peer_access (
    user_id INTEGER NOT NULL REFERENCES users(id),
    peer_user_id INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, peer_user_id),
    CHECK (user_id <> peer_user_id)
);
CREATE INDEX IF NOT EXISTS idx_peer_access_peer ON peer_access(peer_user_id);

-- 客户端本地网段临时注册表（登录上报；断线清空；alone 不转发）
-- 主键 (user_id, dest_cidr)；换机登录先删该账号全部行再写入
CREATE TABLE IF NOT EXISTS client_lan_registry (
    user_id INTEGER NOT NULL REFERENCES users(id),
    dest_cidr TEXT NOT NULL,                    -- 客户端 local_lans 中的网段
    vpn_ip TEXT NOT NULL DEFAULT '',            -- 上报时该账号 VPN IP 快照
    host_id TEXT NOT NULL DEFAULT '',           -- 可选主机标识（日志/排障）
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, dest_cidr)
);
CREATE INDEX IF NOT EXISTS idx_lan_registry_user ON client_lan_registry(user_id);

-- 托管路由定义（手工维护；不跟注册表自动变）：dest via via_user
-- 访问方在 peer_route_members；本表不再存 accessor
CREATE TABLE IF NOT EXISTS peer_routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dest_cidr TEXT NOT NULL,                    -- 目标网段，如 192.168.0.0/24
    via_user_id INTEGER NOT NULL REFERENCES users(id), -- 下一跳账号（via）
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (dest_cidr, via_user_id)
);
CREATE INDEX IF NOT EXISTS idx_peer_routes_via ON peer_routes(via_user_id);

-- 托管路由访问方：user_id=0 表示全部账号；与指定可并存，解析时有 0 则忽略指定
CREATE TABLE IF NOT EXISTS peer_route_members (
    route_id INTEGER NOT NULL REFERENCES peer_routes(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL,                   -- 0=全部；>0 须为 users.id
    PRIMARY KEY (route_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_peer_route_members_user ON peer_route_members(user_id);

-- schema 版本（迁移标记）
CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
