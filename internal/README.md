# internal/ 包索引

> **改代码去哪**：按功能查下表。完整 CODEMAP、分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)（权威单一来源）。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 | `api/handler_routes.go` |
| 健康 / 审计 / Dashboard / 备份 / 日志 | `api/handler_ops.go`（公开 health 仅 ok+uptime；历史日志 items 再脱敏）；Dashboard 字段 `health/dashboard.go` |
| 托管路由 / 互访 / LAN 注册 / HTTP 应用生效 | `api/handler_peer_*.go`；写用例 `vpnaccount/peer_write.go`；DTO `readmodel/peers.go` |
| peer dirty / 应用生效（领域） | `vpnaccount/peer_apply.go`（`PeerPolicyApplier`）；重启 WARN `serverapp/boot_api.go` |
| Session Cookie 写入/清除 / 滑动续期 | `api/auth_handlers.go`：`setSessionCookie` / `clearSessionCookie`（Secure/SameSite 对齐）；Touch 重发 |
| API JSON 成功 / pending_apply / items / JSON 体上限 / 方法守卫 | `api/httputil.go` → `writeOK*`/`writePendingApply`/`writeItems*`/`decodeJSONBody`（1MiB）/`requireMethod` |
| WebUI CSP / 页面脚本 | CSP `security/tls_policy.go`（script+style 均 `'self'`）；`web/static/*.js` + `style.css`；显隐 `HaoVPN.setVisible`/`setOverlayOpen` |
| 字符串 Trim+小写 / VPN 子网 hint | `netutil.TrimLower`；`netutil.InferVPNSubnetHint` |
| Windows PowerShell | `RunPS*` + **`ps_snippets.go`**（找网卡/ICS/孤儿模板） |
| Windows IP Helper / 配 IP | `winnet/iphlp_*`、`SetInterfaceIPv4OnIndex`；开关 `config.windows.use_ip_helper` |
| 分流路由 / DNS 写入 | `netstack/route_ops_windows.go`、`dns_windows.go` |
| 分流路由部分失败 | `clientapp/runtime_routes.go`（`route_install`）；零成功硬失败 |
| DNS show 解析 | `winnet.ParseDNSShowOutput` |
| 手动重连 / Soft vs Hard | Soft：`transport/reconnect.go`；Hard：`clientapp/hard_restart.go`；GUI：`reconnect_dns.go` |
| Stop 屏障 / DNS 先恢复 | `engine_lifecycle.go`；`runtime_routes.go` |
| GUI 线程 / 防连点 | `fyne_meta.go`；`engine_stop.go`（`beginEngineOp` + 按钮 Disable） |
| 托盘悬停气泡 | `tray_tooltip.go`（预算 **63**）；Disconnecting；鉴权后早写 VPNIP |
| GUI 自动连接 / 预热重叠 | `clientgui/run.go`：后台 `clientapp.WarmupTun`；立即 `gui_auto_connect` |
| 握手勿解析 Data | `transport.SetOnHandshake`；`tunnel/client_handshake.go` |
| TUN 预热 | `clientapp.WarmupTun`（禁止 GUI→tun）；孤儿清理 `winnet.BuildPrepareWintunOrphanScript` |
| 服务凭据 DPAPI | `clientapp.SaveServiceCredentials`（禁止 GUI→credentials） |
| Windows ICS/加速门面 | `netstack.ConfigureWindows` / `HasICSResidue` 等（`winnet_facade.go`）；clientapp 禁 winnet |
| TUN 名校验 | `config.ValidateTunName` |
| PS `-match` 转义 | `winnet.EscapeRegex` + `EscapeSingleQuoted` |
| 杀开关前缀去重 | `netutil.DedupTrimNonEmpty` |
| 空 local_lans ICS 清理 | `via_exit.go`；`HasICSResidue`；Teardown：`DisableICSPair`→残留再 `DisableAllICS` |
| 短时重试（Listen 等） | `safeutil/retry.go`（`RetryN`、`ExpBackoff`）；长生命周期 goroutine `safeutil.GoSafe` |
| AbsPair / EnsureDir / ACL / 世界可读 | `fileutil/fs.go`、`mkdir.go`、`perm_*.go`（`RestrictToAdminsOnly`、`CheckWorldReadable`） |
| 广告 LAN 禁 VPN 池重叠 | `netutil.ValidateAdvertisedLANNotForbidden`；握手 `tunnel/server_handler.go` |
| GUI 开机自启（计划任务/服务） | `autostart/`（Win SCM+计划任务；Linux XDG/systemd；macOS LaunchAgent/Daemon；`gen.go`；`paths_unix.go` AbsPair） |
| 探针事件 / 封禁 / 豁免 API / WebUI | `api/handler_security_*.go`；`probedefense/guard.go`；banner I/O `transport/probe_banner.go`；哨兵 `dialerr/`；UX `clientapp/dial_errors.go`（直接 autherr）；页 `security_probe.*` |
| 握手/拨号错误分类 | `autherr/classify.go`（`HandshakeCode`/`FromHandshakeCode`/`Is*`）；`clientapp/fatal_auth.go`（直接调 autherr，无薄 Is*）；`probedefense/classify_handshake.go` |
| 拨号哨兵 / TLS 前 banner 常量 | `dialerr/`（唯一源）；禁止在 transport/autherr/probedefense/tunnel 再定义或薄 re-export 同义哨兵 |
| 源 IP 白名单共用 | `netutil/source_ip.go`（wrap `dialerr.ErrSourceDenied`）；`tunnel` 握手与 `probedefense.Guard` 直接调用 |
| 短时重试 / 指数退避 / GoSafe | `safeutil/retry.go`（`RetryN`、`ExpBackoff`）；`safeutil/goroutine.go`（`GoSafe`）；transport/sessionmgr/logstore/singleinstance 生产路径已收口 |
| TLS 帧 / 重连 | `transport/transport.go`、`server.go`、`reconnect.go`（GoSafe、`Conn.Done`、`ExpBackoff`）、`probe_banner.go` |
| 握手服务端编排 | `tunnel/server_handler.go` + `server_handshake_auth.go`（1～3）+ `server_handshake_session.go`（4～7）+ `handshake_reject.go` |
| IP/CIDR 校验 | `netutil/validate_ip.go`（`ValidateIPOrCIDR`）；列表 `ValidateCIDRList` |
| 管理口 TUN 绑定 / listen_tun | `config/server.go` `api.listen_tun`；`serverapp/boot_api.go`；审计 `audit/tun_listen.go` |
| 握手失败线上 code | `tunnel/handshake.go`；客户端 `client_handshake.go` |
| 探针握手拒绝（无 tunnel→probedefense） | `tunnel.ProbeRecorder.OnHandshakeReject` → `probedefense.Guard` |
| 手动封禁（含豁免） | `probedefense/manual_ban.go`（`ManualBanStore`）；API `handler_security_blocks.go` |
| API 领域错误 → HTTP | `api/httputil.go`（`writeDomainError`、`writeAccountNotFound`）；`persist/peer_access_errors.go` |
| 鉴权中间件去重 | `api/auth_handlers.go`（`validateWebSession`） |
| TLS Accept 探针 | `transport/server.go` 握手失败 → `Probe.OnTransportReadError` |
| Session Context | `api/session_context.go`；`requireAuth` 注入后 handler 用 `actorFromRequest` |
| GUI 托盘路由展示 | `clientapp/route_view.go`（`ManagedRouteView`）；`clientgui/tray_routes.go` |
| 握手策略合并 | `vpnaccount/peer_policy.go` → `ResolveClientPolicy`；会话 `sessionmgr` ViaRoutes/PeerAccess |
| 客户端 local_lans / via 出口 | `config/client.go`；握手 `tunnel/`；出口 `clientapp/via_exit.go`；GUI `clientgui/login.go` |
| 服务端 NAT（工控） | `serverapp/boot_tun.go` + `netstack`（`nat_windows.go` / `ics_nat_windows.go`）；配置 `nat.allowed_lan_cidrs` |
| 用户 CRUD / 删号 / 末管理员 | `api/users_crud.go` → `vpnaccount.DeleteAccount` |
| VPN 策略 PATCH / 启禁 | `api/users_vpn.go` → `vpnaccount.ApplyVPNPatch` / `SetAccountEnabled` |
| 登录 / Session / CSRF / 自改密 | `api/auth_handlers.go`；`auth/`（密码 ≤72） |
| CIDR/LAN/列表工具 | `netutil`（含 `ValidateAdvertisedLANNotForbidden`、`StringSlicesEqualTrimmed`） |
| 桌面 GUI / 托盘 / eng 锁 / 管理员门禁 | `clientgui/`（`admin.go` `requireAdmin`；`tray_config.go`；`engine_stop.go`） |
| 布尔查询/表单 | `paginate.ParseBoolQuery`（api 表单与 `persist/settings.go` 共用） |
| TUN / 路由 / DNS / via | `clientapp/runtime.go`、`runtime_policy.go`、`runtime_routes.go`、`runtime_tun.go`、`via_exit.go`；`netstack/`（`forward_`/`nat_`/`ics_nat_`/`route_ops_`/`dns_`/`killswitch_*`） |
| SQLite / 托管 / 注册表 | `persist/store.go`、`peer_*.go`、`lan_registry.go`、`users.go`、`query_*.go` |
| 会话路由 / 横向隔离 | `sessionmgr/route.go`、`route_inbound.go`、`route_lookup.go`、`route_policy.go` |
| 服务端启动阶段 | `serverapp/engine_boot.go` + `boot_*.go` |
| Windows UAC 提权（含空格路径） | `platform/elevate_windows.go`（`EscapeArg`） |
| 日志脱敏 | `logger/redact.go` |

---

## 按包：主要文件（现行）

| 包 | 文件 | 做什么 |
|----|------|--------|
| **dialerr** | `errors.go` / `classify.go` | 拨号哨兵（中文 Error）、banner 常量、共用前缀匹配、FatalDial、TLS bad-record |
| **autherr** | `classify.go` | 分类 + code；子串表与 Is* 共用；依赖 dialerr，不依赖 transport |
| **probedefense** | `guard.go`（`OnHandshakeReject`）/ `classify_*.go` | 探针；实现 tunnel.ProbeRecorder；无 ErrSourceDenied re-export |
| **clientapp** | `dial_errors.go` / `fatal_auth.go` / `engine_*.go` / `hard_restart.go` / `warmup.go` / `via_exit.go` | UX、fatal；HardRestart；WarmupTun；via/ICS（经 netstack 门面） |
| **clientgui** | `run.go` / `tray_*.go` / `reconnect_dns.go` | 托盘/重连调度；禁 tun/winnet/credentials |
| **transport** | `transport.go` / `server.go` / `probe_banner.go` / `reconnect.go` | Conn/Listen GoSafe；banner I/O；重连 Done/ExpBackoff |
| **tunnel** | `server_handler.go` / `server_handshake_*.go` / `handshake*.go` | 握手编排文件簇；源 IP 直接 netutil |
| **netutil** | `source_ip.go` / `strings.go` / `gateway.go` | TrimLower、InferVPNSubnetHint、源白名单 wrap dialerr |
| **winnet** | `ps_snippets.go` / `escape.go` / `options.go` / `iphlp_*` / `ics_*` | PS 模板；EscapeRegex；IP Helper；ICS |
| **netstack** | `forward_`/`nat_`/`ics_nat_`/`route_ops_`；`winnet_facade.go`；killswitch WFP | 转发/NAT/ICS/路由；对 clientapp 的 Windows 门面 |
| **safeutil** | `goroutine.go` / `retry.go` | `GoSafe`、`RetryN`、`ExpBackoff` |
| **api** | `auth_handlers.go` / `httputil.go` / `handler_peer_*.go` | Cookie helpers；`writeDomainError`；HTTP 薄层 |
| **vpnaccount** | `peer_write.go` / `peer_apply.go` / `provision.go` | peer 写+脏标；开户/策略 |
| **serverapp** | `boot_persist.go` … `boot_api.go` | 启动分阶段；可 import api 启动 HTTP |
| **fileutil** | `fs.go` / `mkdir.go` / `perm_*.go` | EnsureDir、AbsPair、ACL |
| **security** | `tls_policy.go` | CSP `script-src`/`style-src` 均 `'self'` |
| **config** | `client.go` / `tun_name.go` | YAML；`ValidateTunName` |
| **logger** | `redact.go` | Authorization / session= 脱敏 |
| **readmodel** | `peers.go` | Peer 视图 DTO |
| **autostart** | `logon_*.go` / `service_*.go` / `gen.go` | 跨平台自启 |

---

## 分层速览

```
clientapp / clientgui / serverapp
    ├── api ──► vpnaccount（含 PeerPolicyApplier + peer_write）/ auth / probedefense
    ├── tunnel ──► tun
    ├── transport ← Probe
    ├── netstack ──► winnet / platform
    ├── maintenance
    └── persist + sessionmgr
netutil / winnet / fileutil / timeutil / paginate / readmodel / security / config / safeutil / dialerr / autherr / credentials
```

完整包一览见 [architecture.md § CODEMAP](../docs/architecture.md#internal-包-codemap)。

> 架构轮次变更摘要只写 [docs/dev-log.md](../docs/dev-log.md)，本文件不重复堆「第 N 轮」。
> 依赖不变量：`clientgui` 禁 tun/winnet/credentials；`clientapp` 禁 winnet（经 netstack 门面）。

---

## 胖包文件簇（任务导向）

### api/

| 文件簇 | 做什么 |
|--------|--------|
| `handler_routes.go` / `handler.go` | 路由注册与 Handler 构造 |
| `auth_handlers.go` / `session_context.go` | 登录、Cookie、CSRF、Session 注入 |
| `users_*.go` | 用户 CRUD / VPN PATCH |
| `handler_peer_*.go` / `handler_lan_*.go` / `handler_peers_*.go` | 托管路由、互访、LAN、dirty/apply |
| `handler_security_*.go` | 探针事件/封禁/豁免 |
| `handler_ops.go` / `monitor_handler.go` | health、审计、备份、日志、监控 |
| `httputil.go` / `formparse.go` | JSON 信封、领域错误、表单解析 |

### persist/

| 文件簇 | 做什么 |
|--------|--------|
| `store.go` / `schema.sql` / `migrate_*.go` | 打开库、schema、迁移 |
| `users.go` / `settings.go` | 用户与设置 |
| `peer_*.go` / `lan_registry.go` | 托管路由、互访、LAN 注册表 |
| `security_store.go` | security_events / ip_blocks / exempt |
| `query_*.go` / `session_store.go` | 列表查询、会话持久化 |

### clientapp/

| 文件簇 | 做什么 |
|--------|--------|
| `engine_*.go` | 状态机、连接、生命周期 |
| `hard_restart.go` / `warmup.go` | HardRestart / WaitDNSReady；WarmupTun（GUI 门面） |
| `dial_errors.go` / `fatal_auth.go` | 拨号 UX、致命鉴权（autherr+dialerr） |
| `runtime_*.go` / `via_exit.go` / `policy_diff.go` | TUN/路由/策略数据面 |
| `credentials.go` / `bootstrap.go` | 凭据门面与启动 |

### probedefense/

| 文件簇 | 做什么 |
|--------|--------|
| `guard.go` | Accept、RecordReject、`OnHandshakeReject` |
| `classify_*.go` / `signatures.go` / `labels.go` | 特征分类与中文标签 |
| `auto_ban.go` / `manual_ban.go` / `exempt.go` | 自动/手动封禁与豁免 |
| `config_from.go` / `ignorable.go` / `errors.go` | 配置映射、可忽略读错误 |

### transport/

| 文件簇 | 做什么 |
|--------|--------|
| `transport.go` / `conn_loops.go` / `frame.go` | Conn、读写循环、帧 |
| `server.go` | ListenTLS、Accept、Probe |
| `probe_banner.go` | TLS 前 banner I/O（哨兵在 dialerr） |
| `reconnect.go` | 自动重连（GoSafe、ExpBackoff、Conn.Done） |
| `config.go` / `mtu.go` | 配置映射、MTU 探测 |
