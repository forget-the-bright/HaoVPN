# 安全加固清单

> 生产环境上线前逐项核对。默认配置已偏安全，本清单防止**交付时误配**。

---

## 1. 管理口暴露

| 检查项 | 要求 | 命令/方式 |
|--------|------|-----------|
| `api.allow_public_bind` | 必须为 `false` | 查看 `server.yaml` |
| `api.listen_hosts` | 不含 `0.0.0.0` / `::`（除非有充分理由且已评估） | 查看 `server.yaml` |
| frp / 防火墙 | **未**映射管理端口 8080 | 检查 frpc 配置 |
| `api.listen_tun` | 若不需要 VPN 内访问管理口，设 `false` | 默认 true 会 bind VPN 网关 IP（**明文 HTTP**） |
| 外网探测 | 公网 IP:8080 不可达 | `curl` 从外网测试 |

### 公开健康探针（有意设计）

`GET /api/v1/health` 与 `/api/v1/system/info` **无需登录**，用于就绪探针与版本定位。

- **health**：仅 `ok` + `uptime_sec`（进程存活）。**不**返回 `db_ok` / `tun_ok` / `nat_ok` / `online_*` / `recent_errors`。
- **Dashboard**（需登录）：完整数据面状态与 `recent_errors`。
- **system/info**：构建版本与展示时区（非敏感）。

若需完全隐藏探针，请在前置反代层限制来源 IP。

---

## 2. 账号与认证

| 检查项 | 要求 |
|--------|------|
| admin 默认密码 | 已修改，非模板初始值（模板为 `changeme12`）；`dev-security-check` 会 WARN |
| 密码强度 | ≥8 且 **≤72**（bcrypt 有效上限），**须含字母与数字**（代码强制） |
| 自改密 | Web `POST /api/v1/password` 须 `old_password` + `new_password`；成功后吊销该用户全部 Web Session |
| 闲置账号 | 禁用或删除（禁用同时踢 VPN + 吊销 Web 会话）；**不可**删除/禁用最后一个启用的管理员（防 Web 锁死） |
| 用户名格式 | 字母数字与 `._-`，1～64；`auth.ValidateUsername` 在 `EnsureAdmin` / `ProvisionWebAccount` 强制 |
| 登录锁定 | `login_max_attempts` / `login_lockout_sec` 已配置；**Web 与隧道分表**，互不影响 |
| `api.trusted_proxy_cidrs` | 生产默认**留空**；仅反代后且 RemoteAddr 命中信任 CIDR 时才解析 X-Forwarded-For（防锁定绕过） |
| `api.secure_cookies` | HTTPS 终止或全站 TLS 时设为 `true`；与 `setSessionCookie`/`clearSessionCookie` 的 Secure/SameSite **必须一致**，否则 logout 删不掉 Cookie |
| 反代 HTTPS | 配置 `trusted_proxy_cidrs` 且反代送 `X-Forwarded-Proto: https` 时自动 Secure + HSTS | nginx 示例见 deploy |
| 会话滑动 | 鉴权 Touch 成功时**重发**同一 session Cookie（滑动续期）；清除须走 `clearSessionCookie` |
| 须改密 | `must_change_password` 时仍可 GET `/api/v1/csrf`（改密页需要）；其它写 API 仍拦 |
| 注销 | **仅 POST** `/api/v1/logout`（须 CSRF）；GET → 405；响应须带与登录时相同属性的过期 Cookie |
| GUI「记住密码」 | 密码**明文**写入 `client.yaml`（0600）；勾选时 GUI 显示警告；备份/同用户可读，生产慎用 |

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
| 互访默认禁止 | `allow_all_vpn_peers: false`；仅白名单 `peer_access` 或托管路由 via 下一跳可横跳 VPN IP |
| 托管路由 | 控制台 `/peers`；禁 `0.0.0.0/0`；删号级联清理 `peer_*` 表 |
| 重连 grace | `reconnect_grace_sec` 默认 60；异 IP 第二端仍拒绝 |
| 工控网段 | `nat.allowed_lan_cidrs` 仅含必要网段 |
| 隧道来源 IP | 可选配置 `tunnel_allowed_source_ips`（客户端出口固定时建议填；空=不限制） |
| 同账号会话 | `vpn.session_policy: reject_second`（默认；已在线则拒绝第二端，避免互踢） |
| 探针防御 | 见下节；WebUI「探针」页 `/security` |

### 4.1 家里 DDNS / 端口映射

仅映射**隧道口**，勿映射管理口。公网扫描属常态：日志出现 `tls: first record…` / `invalid frame length`（实为 HTTP/`GET ` 等魔数）时，由探针防御记入 `security_events` 并可自动写入 `ip_blocks`。

### 4.2 探针防御与安全事件

**行为**：`serverapp` 在存在 Guard 时**始终**挂到 `transport.Config.Probe`。Accept 时查封禁 → **TLS 握手失败亦记事件**（HTTPS 扫描）→ 非法帧/握手拒绝分类 → 窗口内计数自动封。

**`enabled` / `record_events` / `auto_ban` 真值表**（第十八轮对齐代码）：

| 开关 | 控制范围 |
|------|----------|
| `record_events` | 是否写 `security_events`（含 ban hit、manual ban 审计事件、TLS/帧/握手 rejected） |
| `enabled` | 探针**自动**路径：`RecordReject` 是否参与 **maybeAutoBan** 计数（TLS/帧/Accept 拒绝） |
| `auto_ban` | 窗口达阈值是否写 `ip_blocks`（`maybeAutoBan` 内，还须 `enabled=true`） |

示例：`enabled=false` + `record_events=true` → TLS 拒绝**仍落库**，但**不**触发自动封禁计数。

**配置**（`security.probe_defense` + 相关）：

| 字段 | 含义 |
|------|------|
| `enabled` | 探针自动路径是否参与 **auto-ban 计数**（`RecordReject` → `maybeAutoBan`）；YAML 显式 `false` 永不被默认改回 |
| `record_events` | 是否写 `security_events`（所有探针相关落库的总开关） |
| `auto_ban` | 是否自动写 `ip_blocks` |
| `ban_after_events` / `ban_window_sec` | 阈值与窗口 |
| `ban_duration_sec` | 封禁秒；`0`=永久 |
| `event_retention_days` | 事件保留天（过期 `ip_blocks` 清理**不依赖**本项，由 retention 独立执行） |
| `ignore_signatures_for_ban` | 不计入自动封的特征（默认含 `auth_failed`、`connection_reset`、`unexpected_eof`） |
| `ban_exempt_ips` | **封禁豁免** IP/CIDR；启动导入 DB，与 WebUI 动态条目合并；列表内 IP **永不**自动/手动封禁，且不受 `ip_blocks` 影响 |
| `allow_plaintext_private_keys` | （`security` 段）`true` 时兼容库内明文私钥；**生产必须 false** |

与 **`tunnel_allowed_source_ips` 的区别**：后者为**隧道接入白名单**（非空时仅允许列表内 IP 连隧道）；`ban_exempt_ips` 仅豁免封禁，不限制其它 IP 接入。

与审计日志的区别：`audit_logs` 记管理员操作；`security_events` 记隧道口扫描/握手拒绝。管理端：`/security`；API：`/api/v1/security/events|blocks|exempts`（含 `*_zh` 中文字段）。

**手动封禁 `POST /api/v1/security/blocks`**（须 CSRF）：

```json
{ "ip": "1.2.3.4", "reason": "端口探测", "duration_sec": 604800 }
```

| `duration_sec` | 含义 |
|----------------|------|
| **省略** | 使用 `probe_defense.ban_duration_sec`（自动封禁同源默认） |
| **0** | 永久（`expires_at = NULL`） |
| **> 0** | 指定秒数；须 ≥ 60，上限 10 年（315360000 秒） |

WebUI 探针页预设：1 小时～5 年、永久、自定义（月按 30 天、年按 365 天）；**默认选中 1 周**。审计 `probe_ban_manual` metadata 含 `duration_sec`（或 `default` / `permanent=true`）。

**封禁豁免**（`GET/POST/DELETE /api/v1/security/exempts`，须 CSRF）：

```json
{ "ip": "203.0.113.10", "note": "办公室出口" }
```

- 支持单 IP 或 CIDR；添加后自动 `ReloadBanExempt`；若该 IP 已在封禁表则**自动解封**。
- **DELETE** 路径参数支持 CIDR（如 `203.0.113.0/24`，URL 编码后含 `/`）；服务端用 `ValidateIPOrCIDR` 校验，**勿**以 `/` 误判非法。
- 审计：`probe_exempt_add` / `probe_exempt_remove`。
- WebUI「探针」页「封禁豁免」卡片可动态维护；`server.yaml` 的 `ban_exempt_ips` 启动时幂等导入（`source=yaml_import`）。

**客户端 TLS 前拒绝提示**：

| 服务端写出 | 客户端哨兵 | 用户提示要点 |
|------------|------------|--------------|
| `HAOVPN:IP_BANNED` | `dialerr.ErrIPBanned` | 「IP 已被服务端封禁…」；停重连 |
| `HAOVPN:SOURCE_DENIED` | `dialerr.ErrSourceDenied` | 不在 `tunnel_allowed_source_ips`；停重连 |
| （晚到明文 / 非隧道口） | `dialerr.ErrPlaintextBeforeTLS` | 双因：可能封禁或连错端口；停重连 |
| 无 banner 仅 Close | `dialerr.ErrClosedBeforeTLS` | 可重试；勿当成已封禁 |

实现要点：服务端先 `WriteRejectBanner` 再关连接，记库异步（`safeutil.GoSafe`）；客户端 peek ≈ 250ms。哨兵定义在 `dialerr/`；I/O 在 `transport/probe_banner.go`；UX 在 `clientapp/dial_errors.go`。

握手失败另有 JSON `handshake_err.code`（`autherr.Code*`）：新客户端用 `FromHandshakeCode` 还原哨兵（`errors.Is`）；旧服务端无 code 时回退文案分类。

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

### 4.3 ExitLAN / local_lans 信任边界

- 客户端可上报 `local_lans`（须 **RFC1918** 且前缀 **≥ /16**，禁 `0.0.0.0/0`）；写入 `client_lan_registry` 并进入会话 `ExitLANs`（允许该源入站回程校验）。
- **Windows ICS 与注册表**：新客户端**仅握手**上报 `local_lans`（**已确认**：post-auth `lan_registry_sync` 曾导致 decrypt/replay，已去除）。多 LAN `skipped` 靠 `icsHint`。旧客户端若仍发 sync：服务端限速 + prune，**勿 Kick via 自己**。纵深：Conn 绑定 / Done / Decrypt commit / TUN Connected 前静默上送。PreferVPN/SkipAsSource：冷启/复用主路径为 Go iphlp（`PreferVPNAfterSoftIPReplace`），**保留主机 /24**；纠正错源而非 replay。
- **禁止与 VPN 地址池重叠**：`local_lans` 不得与 `vpn.subnet`（及同池前缀）相交。否则 via 可把 VPN 网段广告为 ExitLAN，再经 hub 旁路 `peer_access` 伪造他户 VPN 源。服务端握手用 `netutil.ValidateAdvertisedLANNotForbidden` 拒绝；过宽/重叠记 `lan_cidr_reject`，不入库。
- **ExitLAN → 其他账号 VPN IP 的 hub 直转**仅当该会话是「已应用托管路由」中的 **via**（`sessionmgr.viaIndex`）。非 via 即使广告了 LAN，也不得绕过 `peer_access`。
- **软重连 / replay**：顶替须 Conn 身份绑定入站 + Close 后排空 `Done`；Decrypt 仅 Open 成功后提交防重放；客户端 Connected 前不上送 TUN。见 [troubleshooting.md](troubleshooting.md) replay 行。
- **应用生效语义**：托管路由/互访/托管 DNS 变更只写库并打 dirty；须点「应用生效」才对**当前在线**受影响账号 `IncrementPolicyVer`+踢线（离线下次握手生效；大批量限速）。成员收窄时 dirty=**旧∪新**；DNS 仅改排除时 dirty=排除对称差。apply 仅清除**本次成功**的 dirty；失败或并发新增保留 pending，避免 UI 伪「已应用」。

### 4.4 管理审计动作 / 目标字典

WebUI `/audit` 展示 `英文码（中文）`；用户目标为 `用户名 (#id)`。权威实现：`internal/audit/labels.go`（改码须同步本表）。

#### 动作 action

| 英文码 | 中文 |
|--------|------|
| `login` | 登录 |
| `login_failed` | 登录失败 |
| `logout` | 退出登录 |
| `change_password` | 修改密码 |
| `account_create` | 创建账号 |
| `account_delete` | 删除账号 |
| `user_enable` / `user_disable` | 启用/禁用账号 |
| `kick_account` | 踢线 |
| `admin_reset_password` | 管理员重置密码 |
| `policy_change_kick` | 策略变更踢线 |
| `config_export` | 导出客户端配置 |
| `db_backup` | 数据库备份 |
| `management_public_bind_enabled` | 管理口公网绑定已开启 |
| `peer_route_create` / `delete` / `members` | 托管路由增删/访问方 |
| `peers_apply` | 应用托管路由 |
| `peer_access_add` / `remove` | 互访白名单 |
| `vpn_peers_policy` | 全局互访策略 |
| `probe_ban_manual` / `probe_unban` | 手动封禁/解封 |
| `probe_exempt_add` / `probe_exempt_remove` | 添加/移除封禁豁免 |

#### 目标 target_type

| 英文码 | 中文 |
|--------|------|
| `user` | 用户 |
| `system` | 系统 |
| `peer_route` | 托管路由 |
| `peer_policy` | 互访策略 |
| `security` | 安全策略 |
| `ip` | IP |

### 4.5 敏感下载须 POST + CSRF

`POST /api/v1/backup`、`POST /api/v1/users/{id}/export` 与 `export.zip` 须 Session + CSRF（防 SameSite=Lax 跨站 GET 拖库）。WebUI 用 `HaoVPN.downloadPost`。

---

## 5. 文件与数据

| 检查项 | 要求 |
|--------|------|
| SQLite 权限 | Linux `chmod 600`；仅服务账户可读 |
| server.yaml 权限 | 限制为管理员可读 |
| 私钥/密码 | 日志写路径经 `logger.RedactSensitive`；含 **Authorization**、`session=`；`/api/v1/logs` 历史 **items** 出口再脱敏一层；仍禁止主动打印明文 |
| Windows 凭据/敏感文件 ACL | `fileutil.RestrictToAdminsOnly`（Administrators+SYSTEM）；`CheckWorldReadable` 含 Windows Everyone 检测（health WARN） |
| peer 待应用 | dirty 集**仅内存**：服务重启后控制台「待应用」清空；启动打 WARN（`boot_api.go`）；库内策略已是权威，在线客户端可能仍持旧策略直至踢线/重连 |
| 定期备份 | 按 [deploy.md](deploy.md) 备份策略执行 |

---

## 6. 审计

| 检查项 | 要求 |
|--------|------|
| audit_logs | 启用且可查询 |
| 敏感操作 | 导出配置、踢人、改密均有记录 |
| 时间存库 | SQLite / JSON API / `?since=` **一律 UTC**；`api.display_timezone` **仅**影响 WebUI 页面展示，不改审计契约 |

---

## 7. WebUI 与 CSP

| 检查项 | 说明 |
|--------|------|
| CSP `script-src` | **`'self'` only**：管理页脚本已外置到 `web/static/*.js`。定义见 `security/tls_policy.go`。 |
| 禁止 HTML `onclick=` / `on*=` | **内联事件处理器同样被 CSP 拦截**。按钮须在外置 JS 用 `addEventListener` / `.onclick=`；退出登录用 `data-action="logout"`（`app.js`）。回归：`TestEmbeddedTemplatesNoInlineEventHandlers` 等。 |
| CSP `style-src` | **`'self'` only**（第二十二轮）：内联 `style=` 与 JS `el.style.*` 已外置到 `style.css` + `.hidden`/`.is-open`；显隐用 `HaoVPN.setVisible` / `setOverlayOpen`。回归：`TestSecurityHeadersScriptAndStyleSelf`、`TestEmbeddedTemplatesNoInlineStyle`。 |
| 新页面 | 禁止在 HTML 内写 `<script>` 业务逻辑、`onclick=`、`style=`；新增 `static/<page>.js` 与必要时扩展 `style.css`。 |

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

## 10. Windows 服务凭据（DPAPI）

客户端 `--service` 可将账号密码存入本机凭据文件（`credentials` 包，`CRYPTPROTECT_LOCAL_MACHINE`）。

| 说明 | 含义 |
|------|------|
| 威胁模型 | **机器级**保护：本机任意能读凭据文件的本地主体均可解密；非用户绑定 DPAPI |
| 为何如此 | 服务账户无交互桌面，须 LocalMachine 才能在开机自启时读密 |
| 运维建议 | 凭据文件写后 `RestrictToAdminsOnly`；生产机勿开共享登录；勿把凭据文件拷到非受信主机 |
| GUI 托盘服务自启 | 启用前 `clientapp.SaveServiceCredentials`；与 CLI `--service` 共用服务名 `HaoVPNClient` |
| yaml 明文密码 | `remember_password` / `gui.auto_connect` 依赖 `client.yaml` 明文；限制文件 ACL（**User DPAPI 仍听安排；架构第 26～29 轮明确未做**） |
| TUN 名 | `config.ValidateTunName`：仅 `[A-Za-z0-9_-]{1,64}`，降低 netsh/PS 注入面 |
| PS 嵌入转义 | `-match`：`EscapeRegex` 再 `EscapeSingleQuoted`；`-eq`/NetNat Name·prefix：仅 `EscapeSingleQuoted`（见 `nat_windows.go`、`ps_snippets.go`） |
| 可取消 PowerShell | Stop/HardRestart：`RunPSOneShotContext` / `RunPSBestEffortContext` / `Setup(ctx)`；日志键 `ps_kill`、`ics_abort` |
| 静态资源 | `handleStatic` 对路径 `path.Clean` 并拒绝 `..` |

`ResolveCredentials`：YAML 已有 `username` 但密码空时，仍可从服务凭据库补密码。

---

## 10.1 客户端 Windows 网卡加速（`windows` 段）

```yaml
windows:
  use_ip_helper: true   # 默认 true：读/写优先 IP Helper；失败回退 netsh/route
```

| 项 | 安全结论 |
|----|----------|
| `use_ip_helper` | 与现网同级管理员权限；无额外提权；失败回退不降权 |
| （已移除）`ps_resident` | 曾长驻 Bypass 管理员 PowerShell；与 CIM/ICS 不兼容且无加速，**代码与配置均已删除** |
| 默认 | Helper 开；PowerShell 一律一进程一脚本 |

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

*最后更新：2026-09-01 · replay/lan_registry 正确性 + 文档治理 / VERSION 0.1.3*
