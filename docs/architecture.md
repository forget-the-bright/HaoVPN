# HaoVPN 架构与 CODEMAP

本文是重构后的**包导航单一来源**：分层、依赖规则、改代码去哪找。

> **文档约定**：进度与轮次摘要只写 [dev-log.md](dev-log.md)；接手入口见 [记忆.md](../记忆.md)；部署/排障/加固各管一块，勿在此重复操作手册。  
> 最近一轮（第十七轮，2026-08-30）：Cookie Secure/SameSite + Touch；`vpnaccount.PeerPolicyApplier`；WebUI 脚本外置 + CSP `script-src 'self'`；叶子 `RetryN` / ACL；Close 竞态与 viaIndex 稳定排序。更早轮次见 dev-log。

---

## 分层

```
cmd/client, cmd/client-gui, cmd/server   # 入口：flag、单实例、提权（GUI UI 在 clientgui）
        │
        ▼
clientapp / clientgui / serverapp        # 应用编排与桌面 UI
        │
        ├── api ──► vpnaccount           # HTTP 薄层；VPN 写经 ApplyVPNPatch；peer 经 PeerPolicyApplier
        ├── tunnel ──► tun               # 握手协议；ServerHandler 持有 tun.Device
        ├── transport                    # TLS-TCP 帧（Probe 钩子）
        ├── probedefense                 # 探针/封禁（有 Guard 即挂载 Accept）
        ├── netstack ──► platform        # 路由/DNS/杀开关；无窗口子进程
        ├── maintenance                  # 数据保留后台（serverapp 启动，与 api 解耦）
        └── persist, auth, sessionmgr    # 存储与会话（account_online 哨兵在 auth）
        │
        ▼
netutil, winnet, paginate, security, config, fileutil, timeutil, readmodel  # 叶子工具
```

---

## 改代码去哪（FAQ）

| 需求 | 目录 / 文件 |
|------|-------------|
| 新增管理 API | `api/users_crud.go` + `handler_routes.go`；业务 `vpnaccount/` |
| API JSON 错误 / 方法守卫 / 表单 / JSON∪表单 / 路径 ID / 内部错误 | `api/httputil.go`（`requireMethod`、`parseFormOrError`、`decodeJSONOrForm`、`parsePathID`、`writeInternalError`、`writeOK`…） |
| 管理 API 监听绑定 | `api/handler_listen.go`（`StartAllListeners`、`FormatBoundAddrs`） |
| CSRF Token | `api/auth_handlers.go`（`handleCSRF`）；比较 `auth/session.go`（常量时间）；`must_change` 仍可 GET `/api/v1/csrf` |
| Session Cookie 写入/清除 / 滑动续期 | `api/auth_handlers.go`：`setSessionCookie` / `clearSessionCookie`（Secure/SameSite 对齐）；Touch 成功时重发 Cookie |
| 公开 health vs Dashboard | `api/handler_ops.go`：公开 health **仅** `ok`+`uptime_sec`；Dashboard 有 db/tun/nat/online/recent_errors |
| API JSON 成功信封 / pending_apply / items 列表 | `api/httputil.go`（`writeOK`、`writeOKWith`、`writePendingApply`、`writeItems`、`writeItemsTotal`、`parseFormInt64`、`parseQueryInt64`、`decodeJSONBody` 体限 1MiB） |
| 托管路由 / 互访 / LAN 注册 / 应用生效 | HTTP：`api/handler_peer_routes.go`、`handler_peer_access.go`、`handler_lan_registry.go`、`handler_peers_apply.go`、`handler_peers_dirty.go`；**领域** `vpnaccount.PeerPolicyApplier`（`peer_apply.go`）；DTO `readmodel/peers.go` |
| peer dirty / 应用生效（非 HTTP） | `vpnaccount/peer_apply.go`（`PeerPolicyApplier`）；重启清空内存脏集，启动 WARN 见 `serverapp/boot_api.go` |
| 管理面 Listen 重试 | `safeutil.RetryN`（`safeutil/retry.go`）；挂点 `serverapp/boot_api.go` |
| WebUI CSP / 页面脚本 | CSP：`security/tls_policy.go`（`script-src 'self'`；`style-src` 仍可 unsafe-inline）；脚本 `web/static/*.js` |
| 广告 LAN 禁与 VPN 池重叠 | `netutil.ValidateAdvertisedLANNotForbidden`；握手挂点 `tunnel/server_handler.go` applyLANRegistry |
| 末管理员保护 | `vpnaccount`（`ErrLastAdmin`）+ `persist.CountEnabledAdmins` |
| VPN 策略 PATCH / 启禁 | `vpnaccount/patch.go`（`ApplyVPNPatch`）、`enable.go`（`SetAccountEnabled`）；禁用吊销 Web 会话 |
| 删 VPN 账号（踢线+释 IP） | `vpnaccount/delete.go` |
| 自改密（须 old_password）/ 须改密 / 吊销会话 | `auth/password_ops.go`、`session.go`（`LogoutAllForUser`）；`api/auth_handlers.go` |
| 登录/握手哨兵错误（含账号已在线） | `auth/errors.go`；客户端致命判定 `clientapp/fatal_auth.go` |
| 用户名格式 | `auth/username.go`（`ValidateUsername`）；开户/EnsureAdmin 强制 |
| Web/隧道分表锁定 | `auth/lockout.go` |
| 探针防御 / 封禁表 | `probedefense/guard.go`；挂载 `serverapp/engine_boot.go`；API `handler_security.go`；UI `/security` |
| 远端 host:port 拆分 | `netutil/hostport.go`（`SplitRemoteAddr`） |
| CIDR 规范化 / 广告 LAN / 列表合并 | `netutil`：`NormalizeCIDROrHost`、`NormalizeCIDRList`、`AppendCIDRUnique`、`ValidateAdvertisedLAN`、`ValidLANCIDRs`、`ForbidDefaultRoute` |
| ExitLAN / via 回程隔离 | `sessionmgr/route.go`（仅 viaIndex 命中才旁路 peer_access）；`persist/lan_registry.go` |
| 审计文案中文 | `audit/labels.go`；API enrichment `handler_ops.go`；对照表 [security-hardening.md](security-hardening.md) |
| 备份 / 导出客户端包 | **POST** `/api/v1/backup`、`/users/{id}/export(.zip)`（须 CSRF）；`HaoVPN.downloadPost` |
| 配置秒 → Duration | `timeutil/duration.go`（`Seconds`） |
| 明文私钥兼容 | `security.allow_plaintext_private_keys`；`tunnel.ServerHandler.AllowPlaintextPrivateKeys` |
| 分页 limit/offset / bool 查询 | `paginate/parse.go`（`ParseLimitOffset`、`ParseBoolQuery`）、`clamp.go` |
| 默认 IP 租约秒数 | `persist/constants.go`（`DefaultIPLeaseSec`，与 schema 同源） |
| 客户端 IP / 反代 XFF | `api/httputil.go`（`resolveClientIP`、`api.trusted_proxy_cidrs`） |
| 日志脱敏 | `logger/redact.go`（首选）；`security.Redact` 为防 logger↔security 循环的薄委托 |
| 密码强度 | `auth/password.go`（`ValidatePasswordStrength`：≥8、≤72、字母+数字） |
| 短时重试（Listen 等） | `safeutil/retry.go`（`RetryN`） |
| 字符串切片 Trim 比较 | `netutil/slices.go`（`StringSlicesEqualTrimmed`） |
| 审计/连接事件/日志/过期封禁保留 | `maintenance/retention.go`（封禁 prune 与事件保留解耦）；默认天数 `config.DefaultRetentionDays` |
| 监控 online/accounts/events | `api/monitor_handler.go`；JOIN `persist/query_monitor.go` |
| 安全事件/封禁 SQL | `persist/security_store.go` |
| 用户/审计/事件列表 SQL | `persist/query_users.go`、`query_audit.go`、`query_events.go` |
| 改握手/策略下发 | `tunnel/handshake.go`, `server_handler.go`（OK 发送失败回滚会话） |
| 客户端拨号/重连/致命鉴权 | `clientapp/engine_lifecycle.go`（`protectForReconnect` 保留数据面）、`engine_connect.go`、`fatal_auth.go`、`credentials.go` |
| 客户端策略差分 / via 指纹 | `clientapp/policy_diff.go`、`runtime.go`（`applyPolicy`）、`via_exit.go` |
| 桌面 GUI（Fyne） | `internal/clientgui/`：托盘配置 `tray_config.go`；路由 `tray_routes.go`；异步退出 `engine_stop.go`；服务接管 `service_takeover.go` |
| GUI 登录/服务自启 | `internal/autostart/`：Win 计划任务+SCM；Linux XDG+systemd；macOS LaunchAgent/Daemon；生成物 `gen.go`；CLI/GUI SCM **只经本包** |
| 服务端启动流程 | `serverapp/engine.go`、`engine_boot.go`、`boot_*.go`（persist/ippool/session/tun/tunnel/api）、`engine_shutdown.go` |
| YAML 默认值/校验 | `config/client.go`、`server.go` |
| 默认 TLS 证书路径 | `config/paths.go`（`DefaultServerCertPath`、`ResolveServerCertPath`） |
| 导出客户端 YAML | `config/client_export.go`；ZIP 在 `api/export_zip.go`；HTTP 附件 `writeAttachment` |
| GUI 写回 client.yaml | `config/client_yaml_patch.go`（`SaveClient`）；Node 原语 `yaml_node.go` |
| CIDR/地址/IPv4 工具 | `internal/netutil/`（含 `ValidLANCIDRs`、`NormalizeCIDRList`、`ValidateAdvertisedLAN`） |
| SQLite / RFC3339 / Seconds | `timeutil/sqlite.go`、`rfc3339.go`、`duration.go` |
| WebUI 展示时区 | `api.display_timezone` → `timeutil/timezone.go`；`GET /api/v1/system/info`；前端 `HaoVPN.formatTime` |
| 发送队列深度 | `vpn.send_queue_size` / 客户端 `server.send_queue_size` → `transport.MaxQueueSize`（`netutil.ClampSendQueueSize`） |
| 敏感文件原子写 / 目录 / Exists / AbsPair / ACL | `fileutil`：`WriteFileAtomic`、`EnsureDir`/`EnsureParentDir`、`ExecutableDir`、`Exists`、`AbsPair`、`CheckWorldReadable`、`RestrictToAdminsOnly` |
| Web/API 读模型 | `readmodel/`（`types.go`、`monitor.go`、`audit.go`、**`peers.go`** PeerRoute/Access/LANRegistry 视图） |
| Dashboard 字段 | `health/dashboard.go`（`DashboardMap`；公开 health 仅 ok+uptime） |
| Windows 路由/DNS/杀开关 | `netstack/` + `winnet/` |
| 无窗口 route/netsh 子进程 | `platform/`（`CommandOutputError`） |
| 客户端单实例 | `singleinstance/`（`lock.go` + `coord.go`，127.0.0.1 TCP） |
| TUN / Wintun DLL | `tun/`、`tun/wintundll/` |
| TLS / 数据密钥 | `security/` |
| WebUI 静态资源 | `web/embed.go` |
| 包索引（任务导向 FAQ） | [internal/README.md](../internal/README.md) |

---

## cmd/ 入口

| 目录 | 职责 |
|------|------|
| `cmd/server` | `-c server.yaml` → `serverapp.New(cfg, path).Run()` |
| `cmd/client` | CLI 拨号、单实例、`--service` → `clientapp` |
| `cmd/client-gui` | flag / UAC / 单实例 / 主题 → `clientgui.Run` |

---

## internal/ 包 CODEMAP

| 包 | 职责 | 关键文件 | 依赖 |
|----|------|----------|------|
| **clientapp** | CLI/GUI 共用拨号引擎；增量 applyPolicy；via 出口 | `engine_*.go`, `runtime.go`, `runtime_policy.go`, `runtime_routes.go`, `runtime_tun.go`, `policy_diff.go`, `via_exit.go`, `credentials.go`, `fatal_auth.go`, `service_windows.go`, `service_other.go` | config, transport, tunnel, netstack, netutil, auth |
| **clientgui** | Fyne UI；托盘配置/路由；异步 Stop；服务接管 | `run.go`, `tray.go`, `tray_config.go`, `tray_routes.go`, `service_takeover.go`, `engine_stop.go`, `admin.go`（`requireAdmin`） | clientapp, autostart, config |
| **autostart** | 登录自启 + 开机无界面：Win SCM/计划任务；Linux XDG/systemd；macOS LaunchAgent/Daemon | `gen.go`（ExecStart 空格引号）、`paths_unix.go`（`AbsPair`）、`logon_*.go`, `service_*.go`, `stub_other.go` | brand, fileutil, logger |
| **serverapp** | 服务端启动编排 | `engine.go`, `engine_boot.go`, `boot_persist.go`, `boot_ippool.go`, `boot_session.go`, `boot_tun.go`, `boot_tunnel.go`, `boot_api.go`（含 peerDirty 重启 WARN）、`engine_shutdown.go` | api, tunnel, transport, probedefense, tun, netstack, vpnaccount, maintenance, safeutil |
| **api** | HTTP 管理 API + WebUI | `handler_*.go`（peer 已拆）、`httputil.go`、`auth_handlers.go`（Cookie helpers）、`users_*.go`；账号写经 vpnaccount；peer dirty/apply 经 `PeerPolicyApplier` | auth, vpnaccount, persist, probedefense, readmodel, config, netutil |
| **readmodel** | Web/API 读模型 DTO | `types.go`, `monitor.go`, `audit.go`, `peers.go` | timeutil |
| **paginate** | 分页 / bool 查询 | `clamp.go`, `parse.go`（`ParseLimitOffset`、`ParseBoolQuery`；表单/settings 共用） | — |
| **maintenance** | 数据保留后台 | `retention.go`（`GoSafe` 启动；封禁 prune 独立） | persist, logstore, config, safeutil |
| **fileutil** | 父目录 / EnsureDir / 原子写 / exe / Exists / AbsPair / ACL | `mkdir.go`, `atomic.go`, `exe.go`, `fs.go`, `perm_*.go` | — |
| **timeutil** | SQLite UTC + RFC3339 + Seconds + 展示时区 | `sqlite.go`, `rfc3339.go`, `duration.go`, `timezone.go` | — |
| **vpnaccount** | IP 模式、开户、策略合并、启禁、删号、末管理员、peer 应用生效 | `service.go`, `peer_policy.go`, `peer_apply.go`（`PeerPolicyApplier`）、`provision.go`, `patch.go`, `delete.go`, `enable.go` | ippool, persist, netutil, auth |
| **tunnel** | 握手协议 | `handshake.go`, `client_handshake.go`, `server_handler.go`, `source_ip.go` | transport, crypto, auth, sessionmgr, netutil, **tun** |
| **transport** | TLS-TCP 帧、重连、Probe | `transport.go`, `config.go`, `conn_loops.go`, `server.go`, `mtu.go`, `frame.go`, `reconnect.go`, `config_from.go` | netutil, timeutil, config |
| **sessionmgr** | 会话与报文路由 | `manager.go`, `register.go`, `kick.go`, `route.go`, `route_inbound.go`, `route_lookup.go`, `route_policy.go`, `stats.go`；托管 via 索引、横向放行、grace 顶替续算 | crypto, netutil, persist, config, auth |
| **probedefense** | 公网探针识别/落库/封禁 | `guard.go`, `labels.go`, `ignorable.go`, `config_from.go` | persist, netutil, config, logger |

| **netstack** | 路由/DNS/杀开关；服务端 NAT 与客户端 via 出口共用 | `route.go`, `route_*.go`, `dns_*.go`, `killswitch_*.go` | winnet, netutil, **platform** |
| **tun** | TUN 抽象（CIDR 解析用 netutil，不导出薄包装） | `tun.go`, `tun_*.go` | winnet, **wintundll**, fileutil, netutil |
| **wintundll**（嵌套于 `tun/wintundll/`） | 嵌入/释放 wintun.dll | `tun/wintundll/ensure.go` | fileutil |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` | platform |
| **netutil** | CIDR/地址/监听/MTU/LAN 广告/列表合并 | `cidr.go`, `addr.go`, `ipmatch.go`, `hostport.go`, `gateway.go`, `listen.go`, `slices.go`（`StringSlicesEqualTrimmed`）, `constants.go` | — |
| **config** | YAML 加载/导出/默认值 | `config.go`, `client_export.go`, `client_yaml_patch.go`, `yaml_node.go`, `paths.go`, `retention.go`, `defaults.go` | netutil, fileutil, brand |
| **security** | TLS、CSP 安全头、密钥加密、绑定自检 | `tls_policy.go`（CSP）、`tls_client.go`, `datakey.go`, `keyenc.go` | netutil, fileutil |
| **persist** | SQLite；托管路由/注册表/迁移 | `store.go`, `schema.sql`, `peer_types.go`, `peer_access.go`, `peer_routes.go`, `peer_route_normalize.go`, `lan_registry.go`（含 host_id 截断）、`migrate_peer_routes.go`, `users.go`, `settings.go`, `query_*.go` | paginate, readmodel, timeutil |
| **auth** | Web Session + 隧道密码 + 分表锁定 + 哨兵 | `errors.go`, `service.go`, `login.go`, `tunnel_login.go`, `session.go`, `lockout.go`, `password.go`, `password_ops.go`, `username.go` | persist |
| **ippool** | VPN IP 池 | `pool.go` | — |
| **health** | 启动自检 + Dashboard | `health.go`, `dashboard.go` | config, persist |
| **logstore** | 结构化历史日志库 | `logstore.go` | paginate, timeutil |
| **audit** | 管理审计 | `audit.go`, `labels.go`（中文标签） | persist |
| **logger** | 分级日志 + 脱敏 | `logger.go`, `redact.go`（Authorization / `session=` 等） | — |
| **safeutil** | GoSafe、Ticker、Shutdown、RetryN | `goroutine.go`, `ticker.go`, `retry.go` | — |
| **crypto** | 隧道加解密 | `wg_crypto.go` | — |
| **credentials** | Windows DPAPI 凭据（写后 RestrictToAdminsOnly） | `windows.go` | fileutil |
| **platform** | UAC（EscapeArg）、无窗口子进程、错误包装 | `elevate_windows.go`, `cmderr.go` | — |
| **singleinstance** | 客户端单实例（TCP 协调） | `lock.go`, `coord.go` | — |
| **brand** | 产品名/路径常量 | `brand.go` | — |
| **version** | 构建版本信息 | `version.go` | — |

每个包均有中文 `doc.go` 说明上下游与关键文件。

---

## 依赖规则

1. **`netstack` 不 import `tun`**：网卡索引经 `winnet`；子进程经 `platform.Command`。
2. **`tunnel` 可 import `tun`**：`ServerHandler.TunDev` 为 `tun.Device`。
3. **`api` 不 import `ippool`**：经 `vpnaccount.Service`；测试 testutil 除外。
4. **`api` 不直接 `UpdateVPNFields` / `SetUserEnabled`**：经 `vpnaccount.ApplyVPNPatch` / `SetAccountEnabled`。
5. **数据保留在 `maintenance`**：`serverapp` 启动 `StartRetentionLoop`（经 `safeutil.GoSafe`）。
6. **分页在 `paginate`**：api、persist、logstore 共用；列表用 `ParseLimitOffset`；bool 用 `ParseBoolQuery`。
7. **`cmd/*` 保持薄**：逻辑在 `clientapp` / `clientgui` / `serverapp`。
8. **默认值**：`config.ApplyDefaults`；传输秒级常量在 `netutil`；保留天数在 `config.DefaultRetentionDays`；默认证书路径在 `config.DefaultServerCertPath`；IP 租约在 `persist.DefaultIPLeaseSec`。
9. **禁止薄 re-export**：直接 import 叶子包。
10. **CIDR/地址纯函数**：仅在 `netutil`；`api.clientIP` 经 `netutil.HostFromAddr`（禁 LastIndex 截端口）。
11. **HTTP DTO**：在 `readmodel`；审计/事件视图不直接序列化 persist 类型。
12. **敏感写盘**：配置/凭据/数据密钥/隧道私钥走 `fileutil.WriteFileAtomic`。
13. **SQLite / API 时间**：统一 `timeutil`；`logstore` 不 import `persist`。
14. **客户端导出 YAML**：生成逻辑在 `config.BuildClientExportYAML`；api 只做 HTTP/ZIP（`writeAttachment`）。
15. **monitor 查询**：经 persist JOIN（`ListMonitorAccountRows` / events JOIN username），禁止逐用户 N+1。
16. **HTTP 成功/分页信封**：无载荷成功用 `writeOK`；标准分页用 `writePage`；禁止手写 `{"ok":true}` / items 信封样板。
17. **账号 allowed_ips 校验**：经 `vpnaccount.ValidateAllowedIPs`（领域别名），api 不直接调 `netutil.ValidateNoFullTunnel`。
18. **探针挂载**：`serverapp` 在 `probeGuard != nil` 时始终设置 `tcfg.Probe`；`Enabled` 只控制自动记录/自动封，封禁表 Accept 始终生效。
19. **握手/登录错误分类**：用 `auth`/`sessionmgr` 哨兵 + `errors.Is`，禁止仅靠中文子串（客户端 fatal、探针 signature 同理）。
20. **Web 与隧道锁定隔离**：`webLockouts` / `tunnelLockouts` 分表，VPN 喷洒不得锁死管理口。

---

## 第十二轮要点（2026-08-29）

- 叶子：`SplitRemoteAddr`、`Seconds`、`fillIPBlock`。
- P0：Probe 始终挂载；握手 OK 失败回滚；改密须旧密码并 `LogoutAllForUser`；`requireAuth` 失败关闭。
- P1：明文钥默认拒绝（`allow_plaintext_private_keys`）；导出不解密；双 lockout；CSRF 常量时间；retention 解耦；sessionmgr 回调/路由加固。

## 第十三轮要点（2026-08-29）

- netutil 收口 CIDR/LAN/广播/远端主机；ExitLAN→对端 VPN 仅 via。
- 管理面 ReadHeaderTimeout 等；备份/导出 POST+CSRF；会话滑动+绝对上限；Accept 源白名单始终生效。
- 审计：`audit/labels.go` + API enrichment；WebUI `码（中文）` / `用户名 (#id)`。

## 第十四轮要点（2026-08-30）

- **叶子**：`ValidLANCIDRs`/`NormalizeCIDRList`/`AppendCIDRUnique` 在 netutil；clientapp 不再为 LAN 校验依赖 persist。
- **哨兵**：`auth.ErrAccountAlreadyOnline`；sessionmgr 兼容别名；clientapp 仅依赖 auth。
- **安全**：公开 health 无 `recent_errors`；末管理员不可删/禁；logout 仅 POST；`ValidateUsername` 下沉；500 稳定「内部错误」。
- **API**：`requireMethod`/`parseFormOrError`/`decodeJSONOrForm`/`parsePathID`；`?online=1` 分页修正。
- **GUI**：`getEngine`/`setEngine`/`takeEngine` 与 `engOpMu` 同锁。

## 第十五轮要点（2026-08-30）

- **SCM 单一真相源**：install/start/stop/uninstall 仅 `autostart`；`clientapp` 只保留 `svc.Run` + CLI 薄封装；修吞错。
- **跨平台自启**：Linux XDG desktop + systemd；macOS LaunchAgent/Daemon；生成物 `gen.go` 可测；Disable/Stop 不再伪成功。
- **叶子边界**：`tun.parseCIDR` 不导出；删 persist LAN 薄包装；`security.Redact` 保留为防循环委托。
- **API**：拆 `handler_peers.go`；`writeOKWith`/`writePendingApply`。
- **安全**：公开 health 仅 `ok`+`uptime_sec`；默认口令测试对齐 `changeme12`；登录脚本外置 `web/static/login.js`（CSP 仍含 unsafe-inline，其它页未迁完）。

## 第十六轮要点（2026-08-30）

- **正确性/安全**：成员替换 dirty=旧∪新；apply 仅清成功 done（防 TOCTOU 伪成功）；`GetPeerRoute` 判空；`local_lans` 禁与 `vpn.subnet` 重叠；via/成员须 VPN 账号；`host_id` 长度上限。
- **叶子收敛**：`paginate.ParseBoolQuery` 统一表单/settings；`httputil` writeItems/parse*Int64/requireMethod；`fileutil.Exists`/`AbsPair`/`CheckWorldReadable`；证书原子写；GUI `requireAdmin`；autostart unix AbsPair。
- **同包拆分**：transport / persist peer_* / sessionmgr route_* / clientapp runtime_* / serverapp boot_*。
- **边界**：peer 视图 DTO→`readmodel/peers.go`；`api/doc.go` 写清 vpnaccount vs persist+sessionmgr；systemd ExecStart 空格引号。

## 第十七轮要点（2026-08-30）

- **安全/正确性**：`setSessionCookie`/`clearSessionCookie` Secure/SameSite 对齐；Touch 重发 Cookie；`must_change` 可取 CSRF；`transport.Conn.Close` 锁拷贝 `onClose`；viaIndex 重建稳定排序；`peer_access` 须已存在 VPN 用户；`decodeJSONBody` 1MiB；密码 ≤72；logger 脱敏 Authorization/`session=`；历史日志 API items 再脱敏；Windows `EscapeArg` / 凭据 ACL / Everyone 检测；`boot_api` peerDirty 重启 WARN。
- **结构**：`fileutil.EnsureDir`/`AbsPair`/`RestrictToAdminsOnly`；`safeutil.RetryN`；`netutil.StringSlicesEqualTrimmed`；**`vpnaccount.PeerPolicyApplier`**（dirty/apply 出 api）；WebUI 页脚本外置，CSP `script-src 'self'`（style 仍 unsafe-inline）。
- **一致性**：`serverapp/boot_*.go` 分阶段启动；与第十六轮拆分命名对齐。

## HTTP API 路由表

注册于 `internal/api/handler_routes.go` `routes()`；写操作须 Session + CSRF。

| 方法 | 路径 | Handler | 鉴权 |
|------|------|---------|------|
| POST | `/api/v1/login` | handleLogin | 公开 |
| GET | `/api/v1/health` | handleHealth（仅 ok + uptime_sec） | 公开 |
| GET | `/api/v1/system/info` | handleSystemInfo | 公开 |
| POST | `/api/v1/logout` | handleLogout（仅 POST+CSRF） | Session+CSRF |
| POST | `/api/v1/password` | handleChangePassword（须 `old_password`+`new_password`；成功吊销会话） | Session |
| GET | `/api/v1/csrf` | handleCSRF | Session |
| GET/POST | `/api/v1/users` | handleUsers | Session |
| * | `/api/v1/users/{id}/…` | handleUserByID | Session |
| GET | `/api/v1/audit` | handleAudit（action_zh / target_username） | Session |
| GET | `/api/v1/dashboard` | handleDashboard | Session |
| POST | `/api/v1/backup` | handleBackup（须 CSRF） | Session+CSRF |
| GET | `/api/v1/logs` | handleLogs | Session |
| GET | `/api/v1/monitor/online` | handleMonitorOnline | Session |
| GET | `/api/v1/monitor/accounts` | handleMonitorAccounts | Session |
| GET | `/api/v1/monitor/events` | handleMonitorEvents | Session |
| GET | `/api/v1/security/events` | handleSecurityEvents | Session |
| GET/POST | `/api/v1/security/blocks` | handleSecurityBlocks | Session |
| DELETE | `/api/v1/security/blocks/{ip}` | handleSecurityBlockByIP | Session |
| GET/POST | `/api/v1/peer-routes` | handlePeerRoutes | Session |
| DELETE | `/api/v1/peer-routes/{id}` | handlePeerRouteByID | Session |
| PUT | `/api/v1/peer-routes/{id}/members` | replacePeerRouteMembers | Session |
| GET | `/api/v1/lan-registry` | handleLANRegistry | Session |
| GET/POST | `/api/v1/peer-access` | handlePeerAccess | Session |
| GET/POST | `/api/v1/peers/apply` | handlePeersApply | Session |
| GET/PUT | `/api/v1/security/vpn-peers` | handleVPNPeersPolicy | Session |

**WebUI**：`/`, `/users`, `/peers`（托管路由 + 本地网段注册表）、`/connections`, `/audit`, `/security`（探针）、`/tools`；`/login` 公开。

**AllowedIPs vs local_lans vs 托管路由（勿混用）**：

| 概念 | 含义 | 存储 / 下发 |
|------|------|-------------|
| **AllowedIPs** | 经**服务端网关/NAT**可达的工控网段（及可选 VPN 子网） | `users.allowed_ips` + 默认 NAT CIDR；握手 `allowed_ips` |
| **local_lans** | 客户端 YAML/GUI **手动**配置的本机后面 LAN；非空才开启 | `client.yaml` → 握手 `local_lans` → 临时表 `client_lan_registry`；客户端 `netstack` via 出口 |
| **托管路由 Managed Routes** | `dest via 客户端`（hub 转 via，via 再出 LAN） | 定义 `peer_routes` + 访问方 `peer_route_members`（`user_id=0`=全部）；握手仅下发**非失效**项 |
| **互访** | 默认可 ping 对方 `vpn_ip/32` 禁止 | `security.allow_all_vpn_peers` / `peer_access`（默认双向）；「应用生效」踢线刷新 |

客户端托盘「本机路由」：本机 TUN（`vpn_subnet`）+ **分流**（AllowedIPs，含 `nat.allowed_lan_cidrs`）+ **对端托管**（`managed_routes`）。「无对端托管」≠ 未装工控路由。

> **流量怎么走、代码落点、与 OpenVPN push/iroute 对照**：见 [traffic-routing.md](traffic-routing.md)。

**失效**：via 离线，或注册表无匹配 `dest` → UI 标失效；握手跳过，不装客户端路由。注册表 alone **不转发**，须管理员从注册表创建托管路由。

出站 `RouteOutbound`：仅 `dst==vpn_ip` 或托管 via 索引；**禁止**用会话 AllowedIPs（NAT）把流量错送回客户端。入站：横向 → via 匹配（优先于 writeTUN）→ 否则 writeTUN（网关 NAT）。

**客户端断线重连与策略应用（差分）**：

| 路径 | 行为 | 代码 |
|------|------|------|
| 临时断线（自动重连） | **保留** TUN / 分流路由 / via·ICS / DNS；若开杀开关则 Enable | `protectForReconnect` |
| 握手后策略 | 比对内容指纹：路由集合差分增删；via 指纹未变则跳过 ICS；完全一致则 `policy_apply mode=noop`；**预判将跑 ICS 时推迟装路由，Setup 后再装一次** | `runtime.applyPolicy`、`policy_diff.go`、`via_exit.go` |
| Stop / 登出 / 策略失败 | **全清**数据面 | `rt.close`、`protectThenClearRoutes` |
| 服务端 | 仍下发完整 `HandshakePolicy`；断线清该用户 LAN 注册表（via 重握手后恢复） | 无需增量帧 |

保留指向 TUN 的路由在断线间隙相当于黑洞，比清路由后回落物理默认路由更不易误出工控网段。

> 探针事件码中英文对照与行为说明见 [security-hardening.md](security-hardening.md)「探针防御与安全事件」。改分类或 Label 须同步该文档与 `probedefense/labels.go`。

---

## 入口示例

**客户端**

```go
cfg, _, _ := config.LoadClient(path)
cfg.Log.InitGlobal()
clientapp.NewEngine(cfg).Start()
```

**服务端**

```go
cfg, _, _ := config.LoadServer(path)
serverapp.New(cfg, path).Run()
```

---

## 测试约定

| 区域 | 测试 |
|------|------|
| fileutil / timeutil / paginate | 各包 `*_test.go`（含 `ParseLimitOffset`） |
| config 导出 YAML / 路径 | `client_export_test.go` |
| persist monitor JOIN | `query_monitor_test.go`、`query_page_test.go` |
| api 导出兼容 / clientIP / harness | `export_zip_test.go`、`httputil_test.go`、`security_test.go`、`testutil_test.go` |
| auth / vpnaccount / sessionmgr | 各包 `*_test.go`（拆文件后同包符号不变） |
| 全量 | `go test ./...`；本机构建 `.\scripts\build-local.ps1` |

---

## 其他目录

| 目录 | 说明 |
|------|------|
| `internal/` | 任务导向 FAQ；见 [internal/README.md](../internal/README.md) |
| `web/` | WebUI embed；见 [web/README.md](../web/README.md) |
| `scripts/` | build-local、build-release、验收 |
| `docs/` | 开发/部署/架构文档 |
