# 安全加固清单

> 生产环境上线前逐项核对。默认配置已偏安全，本清单防止**交付时误配**。

---

## 1. 管理口暴露

| 检查项 | 要求 | 命令/方式 |
|--------|------|-----------|
| `api.allow_public_bind` | 必须为 `false` | 查看 `server.yaml` |
| `api.listen_hosts` | 不含 `0.0.0.0` / `::`（除非有充分理由且已评估） | 查看 `server.yaml` |
| frp / 防火墙 | **未**映射管理端口 8080 | 检查 frpc 配置 |
| 外网探测 | 公网 IP:8080 不可达 | `curl` 从外网测试 |

### 公开健康探针（有意设计）

`GET /api/v1/health` 与 `/api/v1/system/info` **无需登录**，用于就绪探针与版本定位。返回在线数、DB/TUN 状态等**非敏感**摘要；不包含密码、密钥或用户明细。若需隐藏，请在前置反代层限制来源 IP。

---

## 2. 账号与认证

| 检查项 | 要求 |
|--------|------|
| admin 默认密码 | 已修改，非模板初始值 |
| 密码强度 | ≥8 位，**须含字母与数字**（代码强制） |
| 闲置账号 | 禁用或删除 |
| 登录锁定 | `login_max_attempts` / `login_lockout_sec` 已配置 |
| `api.trusted_proxy_cidrs` | 生产默认**留空**；仅反代后且 RemoteAddr 命中信任 CIDR 时才解析 X-Forwarded-For（防锁定绕过） |
| `api.secure_cookies` | HTTPS 终止或全站 TLS 时设为 `true` |

---

## 3. TLS 与证书

| 检查项 | 要求 |
|--------|------|
| 生产证书 | 替换自签证书（`tls.auto_generate: false`） |
| 客户端校验 | `insecure_skip_verify: false` |
| 证书有效期 | 记录在案，到期前更换 |

---

## 4. 隧道与分流

| 检查项 | 要求 |
|--------|------|
| 分流 | `enforce_split_tunnel: true`，不下发 `0.0.0.0/0` |
| 工控网段 | `nat.allowed_lan_cidrs` 仅含必要网段 |
| 隧道来源 IP | 可选配置 `tunnel_allowed_source_ips`（客户端出口固定时建议填；空=不限制） |
| 同账号会话 | `vpn.session_policy: reject_second`（默认；已在线则拒绝第二端，避免互踢） |
| 探针防御 | 见下节；WebUI「探针」页 `/security` |

### 4.1 家里 DDNS / 端口映射

仅映射**隧道口**，勿映射管理口。公网扫描属常态：日志出现 `tls: first record…` / `invalid frame length`（实为 HTTP/`GET ` 等魔数）时，由探针防御记入 `security_events` 并可自动写入 `ip_blocks`。

### 4.2 探针防御与安全事件

**行为**：Accept 时查封禁（**封禁表始终生效**）与可选源白名单 → TLS/非法帧分类落库 → 窗口内计数自动封。`enabled` 只管自动记录与自动封；手动封禁/解封不依赖 `enabled`。心跳读超时**不记**探针。

**配置**（`security.probe_defense`）：

| 字段 | 含义 |
|------|------|
| `enabled` | 自动记录/自动封总开关；YAML 显式 `false` 永不被默认改回 |
| `record_events` | 是否写 `security_events` |
| `auto_ban` | 是否自动写 `ip_blocks` |
| `ban_after_events` / `ban_window_sec` | 阈值与窗口 |
| `ban_duration_sec` | 封禁秒；`0`=永久 |
| `event_retention_days` | 事件保留天 |
| `ignore_signatures_for_ban` | 不计入自动封的特征（默认含 `auth_failed`、`connection_reset`、`unexpected_eof`） |

与审计日志的区别：`audit_logs` 记管理员操作；`security_events` 记隧道口扫描/握手拒绝。管理端：`/security`；API：`/api/v1/security/events|blocks`（含 `*_zh` 中文字段）。

#### 特征 signature（英文码 ↔ 中文）

| 英文码 | 中文含义 |
|--------|----------|
| `account_online` | 账号已在其他设备在线 |
| `auth_failed` | 用户名或密码错误 |
| `source_deny` | 来源 IP 不在白名单 |
| `handshake_reject` | 握手被拒绝 |
| `http_get` | HTTP GET 探测 |
| `http_method` | HTTP 方法探测 |
| `http_blank` | HTTP 空行探测 |
| `amqp` | AMQP 协议扫描 |
| `jrmi` | Java RMI 扫描 |
| `giop` | CORBA/GIOP 扫描 |
| `conn_probe` | CONN 探测 |
| `help_probe` | HELP 探测 |
| `nested_tls` | 套娃 TLS 探测 |
| `frame_invalid` | 非法帧/未知协议 |
| `sslv2` | SSLv2 握手探测 |
| `tls_bad_record` | 非 TLS 首包/坏记录 |
| `tls_cipher_mismatch` | TLS 密码套件不匹配 |
| `tls_old_version` | TLS 版本过旧 |
| `tls_error` | 其它 TLS 错误 |
| `connection_reset` | 对端重置连接 |
| `unexpected_eof` | 对端提前断开 |
| `banned` | 命中封禁 |
| `manual` | 手动封禁 |

> 实现源：`internal/probedefense/labels.go`；改码须同步本表。

#### 阶段 phase

| 英文码 | 中文 |
|--------|------|
| `tcp_accept` | TCP 接入 |
| `tls` | TLS 层 |
| `frame` | 应用帧 |
| `handshake` | 账号握手 |
| `ban_hit` | 封禁命中 |

#### 动作 action

| 英文码 | 中文 |
|--------|------|
| `rejected` | 已拒绝 |
| `banned_hit` | 撞上封禁 |
| `auto_banned` | 已自动封禁 |
| `manual_banned` | 已手动封禁 |

---

## 5. 文件与数据

| 检查项 | 要求 |
|--------|------|
| SQLite 权限 | Linux `chmod 600`；仅服务账户可读 |
| server.yaml 权限 | 限制为管理员可读 |
| 私钥/密码 | 日志与 `/api/v1/logs` 经 **Redact** 脱敏；仍禁止主动打印明文 |
| 定期备份 | 按 [deploy.md](deploy.md) 备份策略执行 |

---

## 6. 审计

| 检查项 | 要求 |
|--------|------|
| audit_logs | 启用且可查询 |
| 敏感操作 | 导出配置、踢人、改密均有记录 |

---

## 7. WebUI 与 CSP

| 检查项 | 说明 |
|--------|------|
| CSP `unsafe-inline` | **有意保留**：零构建链 HTML 模板需内联 script/style；不引外站 CDN。勿随意收紧 CSP，否则登录页白屏。 |

---

## 8. 系统

| 检查项 | 要求 |
|--------|------|
| 运行账户 | 非必要不用 root 登录；服务用专用账户 |
| 防火墙 | 仅开放必要端口（8443 隧道等） |
| 系统补丁 | 现场主机保持更新 |

---

## 9. 对外分发前（发版 / 交付）

| 检查项 | 要求 |
|--------|------|
| LICENSE / NOTICE | `dist/` 发版包须含根目录 [LICENSE](../LICENSE) 与 [NOTICE](../NOTICE)（`build-release` 自动复制） |
| 联系邮箱 | LICENSE §7 联系邮箱已由开发者填写（非占位符） |
| 版权头 | 禁止移除源码或二进制中的版权与许可声明 |
| 商用客户 | 须持书面授权后再商用部署 |

详见 [licensing.md](licensing.md)。

---

## 开发环境例外

开发联调可临时设置：

```yaml
api:
  listen_hosts: ["0.0.0.0"]
  allow_public_bind: true
```

**禁止**将此类配置用于生产。上线前运行：

```powershell
.\scripts\dev-security-check.ps1
```

---

*最后更新：2026-08-29 · 探针防御对照表*
