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

-- schema 版本（迁移标记）
CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
