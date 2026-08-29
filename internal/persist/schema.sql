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

-- schema 版本（迁移标记）
CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
