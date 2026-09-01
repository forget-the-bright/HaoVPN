# HaoVPN 架构与 CODEMAP

本文是重构后的**包导航单一来源**：分层、依赖规则、改代码去哪找。

> **文档约定**：进度与轮次摘要只写 [dev-log.md](dev-log.md)；接手入口见 [记忆.md](../记忆.md)；部署/排障/加固各管一块，勿在此重复操作手册。  
> 最近一轮（2026-09-01）：**架构审计第 29 轮**（ICS/egress PS 集中 / cmd 门面 / PreferVPN 组装 / 死代码 / 文档）；User DPAPI **未做**。见 [dev-log](dev-log.md)。

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
netutil, dialerr, autherr, winnet, paginate, security, config, fileutil, timeutil, readmodel, safeutil  # 叶子工具
```

---

## 改代码去哪（FAQ）

| 需求 | 目录 / 文件 |
|------|-------------|
| 新增管理 API | `api/users_crud.go` + `handler_routes.go`；业务 `vpnaccount/` |
| 管理 API 监听绑定 / TUN 网关 | `api/handler_listen.go`（`StartAllListeners`、`FormatBoundAddrs`）；`api.listen_tun`（默认 true）追加 VPN IP；审计 `audit/tun_listen.go` |
| 握手/拨号错误分类 | `autherr/classify.go`（含 `HandshakeCode`/`FromHandshakeCode`）；拨号哨兵 `dialerr/`；`clientapp/fatal_auth.go`；`probedefense/classify_handshake.go` |
| 拨号/TLS 前拒绝哨兵与 banner | `dialerr/`（唯一源）；I/O 在 `transport/probe_banner.go`；UX `clientapp/dial_errors.go` |
| 握手失败线上 code | `tunnel/handshake.go`（`EncodeHandshakeErrCode`）；客户端 `client_handshake.go` 还原哨兵 |
| 探针握手拒绝挂点 | `tunnel.ProbeRecorder.OnHandshakeReject`；实现 `probedefense.Guard.OnHandshakeReject` |
| API JSON 错误 / 方法守卫 / 领域错误 | `api/httputil.go`（`writeDomainError`、`writeAccountNotFound`、`parsePathSuffixIP`） |
| CSRF Token | `api/auth_handlers.go`（`handleCSRF`）；比较 `auth/session.go`（常量时间）；`must_change` 仍可 GET `/api/v1/csrf` |
| Session Cookie 写入/清除 / 滑动续期 | `api/auth_handlers.go`：`setSessionCookie` / `clearSessionCookie`（Secure/SameSite 对齐）；Touch 成功时重发 Cookie |
| 公开 health vs Dashboard | `api/handler_ops.go`：公开 health **仅** `ok`+`uptime_sec`；Dashboard 有 db/tun/nat/online/recent_errors |
| API JSON 成功信封 / pending_apply / items 列表 | `api/httputil.go`（`writeOK`、`writeOKWith`、`writePendingApply`、`writeItems`、`writeItemsTotal`、`parseFormInt64`、`parseQueryInt64`、`decodeJSONBody` 体限 1MiB） |
| 托管路由 / 互访 / LAN 注册 / 应用生效 | HTTP：`api/handler_peer_routes.go`、`handler_peer_access.go`、`handler_lan_registry.go`、`handler_peers_apply.go`；**写用例** `vpnaccount/peer_write.go` + dirty/apply `peer_apply.go`；DTO `readmodel/peers.go` |
| peer dirty / 应用生效（非 HTTP） | `vpnaccount/peer_apply.go`（`PeerPolicyApplier`）；重启清空内存脏集，启动 WARN 见 `serverapp/boot_api.go` |
| TUN 名校验 | `config.ValidateTunName`（`tun_name.go`）；客户端 Validate 强制 `[A-Za-z0-9_-]{1,64}` |
| TUN 预热（CLI/GUI/服务） | `clientapp.StartWarmupAsync` → `warmupTun` → `tun.WarmupAdapter`；GUI **禁止** import tun；与拨号重叠；Open `reuse from_warmup` |
| CLI 启动契约 | `clientapp/bootstrap.go`：`RunCLI`（预热+FailFast+45s）vs `RunServiceLoop`（持续重连） |
| GUI 引擎启动契约 | `clientapp/engine_bootstrap.go`：`PrepareEngine` + `StartAndWaitFirstAuth`（与 CLI 共用 FailFast/45s/`FormatConnectFailure`） |
| 连接失败 / 拨号 UX 文案 | **首连** `connect_failure.go` → `FormatConnectFailure`；**拨号/TLS** `dial_errors.go` → `FormatDialError`；**已连告警** `connect_warn.go` → `MergeConnectWarns`；停止重连判定 `ShouldStopReconnectOnDial`（勿与 `dialerr.IsFatalDialError` 混淆） |
| 服务凭据（LocalMachine DPAPI） | `clientapp.SaveServiceCredentials` → `credentials.SaveService`；GUI 禁止直接 import credentials |
| Windows 加速/ICS 门面 | `netstack.ConfigureWindows` / `HasICSResidue` / `CleanupICSResidue(Context)` / `RemoveICSAddressesKeepVPN` / `ReplaceTUNIPv4KeepICS`（`winnet_facade.go`）；**不**导出 DisableAllICS/DisableICSPair——Teardown 经 `ics_enable_windows.disableICSPlatform` → `winnet.DisableICSSessionContext`；`clientapp` **禁止**直接 import winnet |
| 入站解密 / 软重连 | `sessionmgr.HandleInbound(userID, conn, …)`（须 Conn 匹配）；`RegisterVPN` Close+`Done` 排空；`crypto.Decrypt` Open 成功后 commit 防重放；客户端 `tunUploadReady`（StateConnected 闸门） |
| LAN 注册表 | 握手 `applyLANRegistry`（新客户端仅此）；旧客户端 post-auth sync → `tunnel/lan_registry_sync.go`（限速 + prune + 勿 Kick via 自己） |
| PS `-match` 转义 | `winnet.EscapeRegex` 再 `EscapeSingleQuoted`；模板 `ps_snippets.go`（含 `PSSnippetNewNetNat`/`Remove`/`EnableIPv4Forwarding`/`GetNetNatMatch`）；编排在 `nat_windows.go` / `forward_windows.go` |
| 记住密码（yaml 明文）债 | 仍明文写 `client.yaml`；User DPAPI **听安排 / 第 26～29 轮未做** — 详见 [security-hardening.md](security-hardening.md) §10 |
| 短时重试 / 可中止轮询 | `safeutil.RetryN` / `ExpBackoff` / **`PollUntil`**；**ctx 取消**用 `IsCanceled` / `Check`；**登录超时文案**用 `IsDeadline`（不含 Canceled） |
| 管理面 Listen 重试 | `safeutil.RetryN`；挂点 `serverapp/boot_api.go` |
| WebUI CSP / 页面脚本 | CSP：`security/tls_policy.go`（`script-src`/`style-src` 均 `'self'`，无 unsafe-inline）；脚本 `web/static/*.js`；样式 `style.css`；显隐 `HaoVPN.setVisible` / `setOverlayOpen` |
| 广告 LAN 禁与 VPN 池重叠 | `netutil.ValidateAdvertisedLANNotForbidden`；握手挂点 `tunnel/server_handler.go` applyLANRegistry |
| 末管理员保护 | `vpnaccount`（`ErrLastAdmin`）+ `persist.CountEnabledAdmins` |
| VPN 策略 PATCH / 启禁 | `vpnaccount/patch.go`（`ApplyVPNPatch`）、`enable.go`（`SetAccountEnabled`）；禁用吊销 Web 会话 |
| 删 VPN 账号（踢线+释 IP） | `vpnaccount/delete.go` |
| 自改密（须 old_password）/ 须改密 / 吊销会话 | `auth/password_ops.go`、`session.go`（`LogoutAllForUser`）；`api/auth_handlers.go` |
| 登录/握手哨兵错误（含账号已在线） | `auth.ErrAccountAlreadyOnline`（`auth/errors.go`）；客户端致命判定 `clientapp/fatal_auth.go`；sessionmgr **不再** re-export 别名 |
| 用户名格式 | `auth/username.go`（`ValidateUsername`）；开户/EnsureAdmin 强制 |
| Web/隧道分表锁定 | `auth/lockout.go` |
| 探针防御 / 封禁表 | `probedefense/guard.go`；分类 `classify_tls.go` / `classify_handshake.go`；常量 `signatures.go`；自动封 `auto_ban.go`；挂载 `serverapp/boot_session.go`；API `handler_security_*.go`；UI `/security` |
| IP/CIDR 校验（单 IP 或 CIDR） | `netutil/validate_ip.go`（`ValidateIPOrCIDR`）；列表 `cidr.go`（`ValidateCIDRList`） |
| 公网管理口绑定启动审计 | `audit/public_bind.go`（`LogPublicBindEnabled`）；`serverapp/boot_persist.go` |
| Session Context（requireAuth 注入） | `api/session_context.go` + `auth_handlers.go`；handler 用 `actorFromRequest` |
| GUI 托盘托管路由展示 DTO | `clientapp/route_view.go`（`ManagedRouteView`）；`clientgui/tray_routes.go` |
| 远端 host:port 拆分 | `netutil/hostport.go`（`SplitRemoteAddr`） |
| CIDR 规范化 / 广告 LAN / 列表合并 | `netutil`：`NormalizeCIDROrHost`、`NormalizeCIDRList`、`AppendCIDRUnique`、`ValidateAdvertisedLAN`、`ValidLANCIDRs`、`ForbidDefaultRoute`、`ProbeIPForCIDR`、`ParseLocalLANsField`、`FilterDNSServersPoison`、`IsVirtualInterfaceName` |
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
| 字符串切片 Trim 比较 | `netutil/slices.go`（`StringSlicesEqualTrimmed`） |
| 审计/连接事件/日志/过期封禁保留 | `maintenance/retention.go`（封禁 prune 与事件保留解耦）；默认天数 `config.DefaultRetentionDays` |
| 监控 online/accounts/events | `api/monitor_handler.go`；JOIN `persist/query_monitor.go` |
| 安全事件/封禁 SQL | `persist/security_store.go` |
| 用户/审计/事件列表 SQL | `persist/query_users.go`、`query_audit.go`、`query_events.go` |
| 改握手/策略下发 | `tunnel/server_handler.go`（编排）+ `server_handshake_auth.go`（1～3）+ `server_handshake_session.go`（4～7）；OK 发送失败回滚会话 |
| 客户端拨号/重连/致命鉴权 | `clientapp/engine_lifecycle.go`（Stop 等 rc 再关 dataplane）、`engine_connect.go`、`fatal_auth.go`、`dial_errors.go`；重连 `transport/reconnect.go`（Stop 等 loop / Dial 门闩） |
| 握手帧 vs Data | `transport`：`SetOnHandshake`；`tunnel/client_handshake.go` 只挂 Handshake；Data 密文不得当 JSON |
| 源 IP 白名单共用 | `netutil/source_ip.go`（`CheckSourceIPAllowed` wrap `dialerr.ErrSourceDenied`）；`tunnel`/`probedefense` 直接调用，无薄包装 |
| 客户端策略差分 / via 指纹 | `clientapp/policy_diff.go`、`runtime.go`（`applyPolicy`）、`via_exit.go` |
| 空 local_lans 与 ICS 清理 | `clientapp/via_exit.go`：`HasICSResidue` → 无残留跳过；hadVia 只 `RemoveICSAddressesKeepVPN`；否则 `CleanupICSResidueContext`（可取消） |
| WinNAT / ICS | `nat_windows.go` + `sku_home`；`ics_egress` + `ics_enable`：有 137→`reuse_live`；无→Force Restart→Enable→**Go iphlp Prefer**；`ICSLifecycle` Disable/Preserve；注册表仅握手 |
| 家庭版 NAT | `winnet.IsWindowsHomeSKU` → 跳过 WinNAT 直进 ICS |
| 启动耗时埋点 | `prepare_orphan skipped` / `dns_snapshot skip_empty` / `ics_egress` / `ics_stage stage=restart\|enable` / `prefer_vpn_light` / `ics=preserve\|disable` |
| Wintun 孤儿 | `winnet.HasWintunOrphanAdapters` + `IsWintunOrphanAdapterName`；有孤儿才 PS Remove |
| GUI 防连点 / Stop 串行 | `clientgui` `beginEngineOp` + `beginNetworkOp` + 按钮 Disable；`stopEnginesSerial`；busy 时 `pendingIntent`+`opGen`（`engine_op_queue.go`）；login_fail busy→排队 logout |
| Fyne.Do 迁移 | 构建 `-tags migrated_fynedo`（`Invoke-GoBuildGui`）+ `clientgui/fyne_meta.go` SetMetadata；仅改 `FyneApp.toml` 对 go build 无效 |
| 配网时心跳 / Stop 打断 policy | `applyPolicy(ctx)`；`Stack.Setup(ctx)` + `RunPS*Context` Kill；**abort 不得 forward_only 吞成功** |
| 手动重连 / Soft vs Hard | Soft：`transport/reconnect.go` + `protectForReconnect`；Hard：`HardRestart`→`StopKeepICS`（`ICSPreserve`）→有 137 则 `reuse_live`；GUI：`reconnect_dns.go`；登录页仍 FailFast |
| GUI 服务接管 | `clientgui/service_takeover.go` + `clientapp/autostart_facade`（ServiceStopAndWait）；`cmd/client-gui` `handleOccupiedInstance` |
| Stop 时 DNS/路由顺序 | `runtime_routes.go`：先 `RestoreDNS(poison…)` 再删路由；`Engine.Stop`：先 `cancel(runCtx)` 再 `rc.Stop`/清数据面 |
| 分流路由部分失败 | `route_install` 日志；零成功硬失败；部分成功 → `LastError` + `partial_routes=true` |
| 同 IP vs 变 VPN IP | `runtime_policy.go`：同 IP 保 dataplane；**变 IP 软换** `vpn_ip_replace_inplace`（`ReplaceTUNIPv4KeepICS` + PreferVPN，**不**拆 via/ICS）；冷启动才 `tun.Open` |
| TUN 上送 / 越权目的 | 公式：`netutil.VPNIPOrInNets`；噪声：`IsTUNNoiseDst` / `IsTUNNoiseForLog` / **`IsTUNNoiseSource`**；限频：`safeutil.AllowEvery`；DNS `/32`→`MergeDNSIntoAllowedIPs` |
| 配 TUN IPv4 / wait | `tun/tun_windows.go` `assignIPv4`（禁止「已有则跳过」）；`winnet.ReplaceInterfaceIPv4` / `ReplaceInterfaceIPv4KeepICS`；埋点 `tun_addrs_before/after` |
| 分流路由 / DNS 写入 | `route_ops_windows.go`（add/del 优先 iphlp）；`dns_windows.go`→首次 `skip_empty`；`RestoreDNS(poison…)` 防旧 VPN IP 写回 |
| Windows 出站快照 | `winnet/egress.go` + `egress_windows.go`（一次 GAA+路由表） |
| PreferVPN / SkipAsSource | 主路径：`applyPreferVPNAfterICS`→`PreferVPNAfterSoftIPReplace`（iphlp scrub + SkipAsSource，**保留主机 /24**）；回退：`PreferVPNSourceWithICSContext`（PS）；**禁止** `ics_prefix_fix` |
| send queue full | `transport.noteSendQueueFull` 限频 5s + drops；根因常在数据面错乱，勿只加大队列 |
| Windows `client.yaml` windows 段 | `config.ClientWindowsSection`：`use_ip_helper`；`NewEngine`→`netstack.ConfigureWindows` |
| 策略分段耗时 | `runtime_policy.go` `policy_apply stage=` |
| 字符串 Trim+小写 / VPN 子网 hint / 网关 | `netutil.TrimLower`；`InferVPNSubnetHint`；`ResolveGateway(handshakeGW, vpnIP)`（yaml gateway 已废除） |
| Windows PowerShell 模板 | `winnet/ps_snippets.go`（找网卡/ICS/WinNAT/孤儿）；`RunPS*` 一律一进程 |
| DNS 恢复（Windows） | `netstack/dns_windows.go` ApplyDNS/RestoreDNS（skip_empty 快照；不再读 netsh QueryInterfaceDNS） |
| 杀开关前缀去重 | `netutil.DedupTrimNonEmpty` |
| 桌面 GUI（Fyne） | `clientgui/` + `fyne_meta.go` + `tray_tooltip.go`；tag `migrated_fynedo` |
| 托盘悬停气泡 | `tray_tooltip.go`（预算 **63**）；Disconnecting；`trayStickyErr`（登录失败无 eng）；busy 时 sticky 优先于「正在断开」 |
| GUI 登录失败收口 | `clientgui/login_fail.go`；HardRestart 失败须 Stop；`engine_intent.go` 抢占 |
| GUI 登录/服务自启 | `clientapp/autostart_facade.go`（编排）；底层 `autostart/`（Win SCM+计划任务；Linux/macOS 见包内）；GUI `tray_config.go` 仅读 UI→调 facade |
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
| Windows 路由/DNS/杀开关 | `netstack/`：`forward_windows.go`、`nat_windows.go`、`ics_egress_windows.go`、`ics_enable_windows.go`、`ics_plan.go`、`route_ops_windows.go`、`dns_windows.go`、`killswitch_windows.go` + `killswitch_wfp_*.go`；底层 `winnet/` |
| 无窗口 route/netsh 子进程 | `platform/`（`CommandOutputError`）；**PowerShell 一律 `winnet.RunPSOneShot*` / `RunPSBestEffort*`** |
| 客户端单实例 | `singleinstance/`（`lock.go` + `coord.go`，127.0.0.1 TCP） |
| TUN / Wintun DLL | `tun/`（含 `WarmupAdapter`；GUI 经 `clientapp.StartWarmupAsync`）、`tun/wintundll/` |
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
| **clientapp** | CLI/GUI 共用拨号引擎；**bootstrap**（RunOptions）；**engine_bootstrap**（PrepareEngine）；增量 applyPolicy；via 出口；HardRestart；Warmup；服务/自启 facade | `bootstrap.go`, `engine_bootstrap.go`, `engine_*.go`, `hard_restart.go`, `warmup.go`, `hooks.go`, `connect_failure.go`, `connect_warn.go`, `dial_errors.go`, `autostart_facade.go`, `runtime*.go`, `via_exit.go` | config, transport, tunnel, dialerr, autherr, netstack, netutil, tun, credentials（**禁** winnet） |
| **clientgui** | Fyne UI；托盘；异步 Stop；HardRestart + intent 抢占 | `run.go`, `tray.go`, `tray_state.go`, `reconnect_dns.go`, `engine_intent.go`, `engine_stop.go`, `login.go`/`login_fail.go` | clientapp, config, netutil（**禁** tun/winnet/credentials/autostart 直接编排） |
| **autostart** | 登录自启 + 开机无界面：Win SCM/计划任务；Linux XDG/systemd；macOS LaunchAgent/Daemon | `gen.go`（ExecStart 空格引号）、`paths_unix.go`（`AbsPair`）、`logon_*.go`, `service_*.go`, `stub_other.go` | brand, fileutil, logger |
| **serverapp** | 服务端启动编排 | `engine.go`, `engine_boot.go`, `boot_persist.go`, `boot_ippool.go`, `boot_session.go`, `boot_tun.go`, `boot_tunnel.go`, `boot_api.go`（含 peerDirty 重启 WARN）、`engine_shutdown.go` | api, tunnel, transport, probedefense, tun, netstack, vpnaccount, maintenance, safeutil |
| **api** | HTTP 管理 API + WebUI | `handler_*.go`、`httputil.go`（`writeDomainError`）、`users_*.go`；账号/peer **写**经 vpnaccount | auth, vpnaccount, persist, probedefense, readmodel, config, netutil |
| **readmodel** | Web/API 读模型 DTO | `types.go`, `monitor.go`, `audit.go`, `peers.go` | timeutil |
| **paginate** | 分页 / bool 查询 | `clamp.go`, `parse.go`（`ParseLimitOffset`、`ParseBoolQuery`；表单/settings 共用） | — |
| **maintenance** | 数据保留后台 | `retention.go`（`GoSafe` 启动；封禁 prune 独立） | persist, logstore, config, safeutil |
| **fileutil** | 父目录 / EnsureDir / 原子写 / exe / Exists / AbsPair / ACL | `mkdir.go`, `atomic.go`, `exe.go`, `fs.go`, `perm_*.go` | — |
| **timeutil** | SQLite UTC + RFC3339 + Seconds + 展示时区 | `sqlite.go`, `rfc3339.go`, `duration.go`, `timezone.go` | — |
| **vpnaccount** | IP 模式、开户、策略、启禁、删号、peer 写+应用生效 | `service.go`, `peer_write.go`, `peer_apply.go`, `peer_policy.go`, `provision.go`, `patch.go`, `delete.go`, `enable.go`, `errors.go` | ippool, persist, netutil, auth |
| **tunnel** | 握手协议（含 handshake_err.code） | `handshake.go`, `handshake_reject.go`, `client_handshake.go`, `server_handler.go`, `server_handshake_auth.go`, `server_handshake_session.go` | transport, crypto, auth, autherr, sessionmgr, netutil, **tun** |
| **transport** | TLS-TCP 帧、重连、Probe、banner I/O | `transport.go`, `config.go`, `conn_loops.go`, `server.go`, `probe_banner.go`, `mtu.go`, `frame.go`, `reconnect.go`（均经 GoSafe） | dialerr, netutil, timeutil, config, safeutil |
| **dialerr** | 拨号/TLS 前拒绝叶子哨兵与 banner 常量 | `errors.go`, `classify.go` | — |
| **sessionmgr** | 会话与报文路由 | `manager.go`, `register.go`, `register_grace.go`, `register_lan.go`, `kick.go`, `route.go`, `route_inbound.go`, `route_lookup.go`, `route_policy.go`, `stats.go`；托管 via 索引、横向放行、grace 顶替续算 | crypto, netutil, persist, config, auth, safeutil |
| **probedefense** | 公网探针识别/落库/封禁；实现 tunnel.ProbeRecorder | `guard.go`（`OnHandshakeReject`）、`exempt.go`, `manual_ban.go`, `classify_tls.go`, `classify_handshake.go`, `signatures.go`, `auto_ban.go`, `errors.go` | persist, netutil, dialerr, config, autherr, logger |
| **netstack** | 路由/DNS/杀开关；Windows 门面（Configure/ICS 探测清理）；`Setup(ctx)`/`Teardown(ctx)` | `forward_windows.go`, `nat_windows.go`, `ics_egress_windows.go`, `ics_enable_windows.go`, `ics_plan.go`, `route_ops_windows.go`, `dns_*.go`, `killswitch_*`, `winnet_facade.go` | winnet, netutil, platform, safeutil |
| **tun** | TUN 抽象（CIDR 解析用 netutil，不导出薄包装） | `tun.go`, `tun_*.go` | winnet, **wintundll**, fileutil, netutil |
| **wintundll**（嵌套于 `tun/wintundll/`） | 嵌入/释放 wintun.dll | `tun/wintundll/ensure.go` | fileutil |
| **winnet** | Windows 网卡/IP Helper/替换式配 IP/netsh/ICS/PS 模板与转义；Context Kill | `ipv4_replace.go`、`iphlp_write_windows.go`、`ps_windows.go`、`ps_snippets.go`、`escape.go`、`ics_*`（`DisableICSSessionContext`） | platform, logger, netutil, brand, safeutil |
| **netutil** | CIDR/地址/监听/MTU/LAN/源 IP/TrimLower/子网 hint | `cidr.go`, `validate_ip.go`, `addr.go`, `source_ip.go`、`gateway.go`（InferGateway/InferVPNSubnetHint）、`strings.go`（TrimLower）、`listen.go`, `slices.go` | dialerr |
| **autherr** | 握手/拨号错误分类 + 线上 code 映射 | `classify.go` | auth, dialerr |
| **audit** | 管理审计 | `audit.go`, `labels.go`, `public_bind.go`, `tun_listen.go` | persist |
| **config** | YAML 加载/导出/默认值/TUN 名校验 | `config.go`, `client.go`, `tun_name.go`（`ValidateTunName`）、`defaults.go` | netutil, fileutil, brand |
| **security** | TLS、CSP 安全头、密钥加密、绑定自检 | `tls_policy.go`（CSP script+style 均 `'self'`）、`tls_client.go`, `datakey.go`, `keyenc.go` | netutil, fileutil |
| **persist** | SQLite；托管路由/注册表/迁移 | `store.go`, `schema.sql`, `peer_types.go`, `peer_access.go`, `peer_routes.go`, `peer_route_normalize.go`, `lan_registry.go`（含 host_id 截断）、`migrate_peer_routes.go`, `users.go`, `settings.go`, `query_*.go` | paginate, readmodel, timeutil |
| **auth** | Web Session + 隧道密码 + 分表锁定 + 哨兵 | `errors.go`, `service.go`, `login.go`, `tunnel_login.go`, `session.go`, `lockout.go`, `password.go`, `password_ops.go`, `username.go` | persist |
| **ippool** | VPN IP 池 | `pool.go` | — |
| **health** | 启动自检 + Dashboard | `health.go`, `dashboard.go` | config, persist |
| **logstore** | 结构化历史日志库 | `logstore.go` | paginate, timeutil |
| **logger** | 分级日志 + 脱敏 | `logger.go`, `redact.go`（Authorization / `session=` 等） | — |
| **safeutil** | GoSafe、Ticker、Shutdown、RetryN、ExpBackoff、IsCanceled/Check | `goroutine.go`, `ticker.go`, `retry.go`, `context.go` | — |
| **crypto** | 隧道加解密 | `wg_crypto.go` | — |
| **credentials** | Windows DPAPI 凭据（写后 RestrictToAdminsOnly） | `windows.go` | fileutil |
| **platform** | UAC（EscapeArg）、无窗口子进程、错误包装 | `elevate_windows.go`, `cmderr.go` | — |
| **singleinstance** | 客户端单实例（TCP 协调） | `lock.go`, `coord.go` | — |
| **brand** | 产品名/路径常量 | `brand.go` | — |
| **version** | 构建版本信息 | `version.go` | — |

每个 `internal/*` 包均有中文 `doc.go`（含嵌套 `clientgui/icons`）。

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
19. **握手/登录错误分类**：优先 `auth`/`dialerr` 哨兵 + `handshake_err.code`（`autherr.FromHandshakeCode`）；禁止仅靠中文子串作为主路径（旧服务端无 code 时才子串兜底）。
20. **Web 与隧道锁定隔离**：`webLockouts` / `tunnelLockouts` 分表，VPN 喷洒不得锁死管理口。
21. **拨号哨兵唯一源**：`dialerr`；`autherr`/`transport`/`netutil`/`clientapp` 不得再 new 同义 `ErrSourceDenied`/`ErrIPBanned`。
22. **`tunnel` 不得 import `probedefense`**：握手拒绝经 `ProbeRecorder.OnHandshakeReject`；分类在 Guard 内完成。
23. **`autherr` 不得 import `transport`**：只依赖 `auth` + `dialerr`。
24. **生产路径长生命周期 goroutine**：须 `safeutil.GoSafe`（transport Conn 循环、ListenTLS、sessionmgr 关旧连接、logstore writer、singleinstance accept）；测试内裸 `go` 可保留。
25. **源 IP 白名单**：仅 `netutil.CheckSourceIPAllowed`；`tunnel`/`probedefense` 禁止薄包装或再 export `ErrSourceDenied`。
26. **Windows PowerShell**：一律 `winnet.RunPSOneShot` / `RunPSOneShotContext` / `RunPSBestEffort` / `RunPSBestEffortContext`（一进程、`-ExecutionPolicy Bypass`；取消 Kill）；禁止业务包 raw powershell；清理失败须 Warn；嵌入参数须 `EscapeSingleQuoted`（`-match` 先 `EscapeRegex`）。
27. **WebUI CSP**：`script-src` 与 `style-src` 均仅 `'self'`；禁止 HTML `style=` / 内联 `<style>` / JS 写 `el.style.*`（显隐用 class + `HaoVPN.setVisible`/`setOverlayOpen`）。
28. **`clientgui` 禁止 import `tun` / `winnet` / `credentials` / 直接 `autostart.*` 编排**：预热经 `clientapp.StartWarmupAsync`；自启经 `autostart_facade`；服务凭据经 `SaveServiceCredentials`。
29. **`cmd/client` / `cmd/client-gui` 读 Windows 服务状态**须经 `clientapp.ServiceAutostartStatus`（禁 direct `autostart.ServiceStatus`）。
30. **`clientapp` 禁止 import `winnet`**：经 `netstack` 门面（`ConfigureWindows` / ICS 探测清理）。
31. **`api` 不直接 `InsertPeerRoute` / `AddPeerAccessPair` 等 peer 写**：经 `vpnaccount.PeerPolicyApplier`（`peer_write.go`）。
32. **TUN 名**：`config.ValidateTunName`；PS `-match` 须 `EscapeRegex` 再 `EscapeSingleQuoted`。
33. **`netstack.Setup(ctx)` / `Teardown(ctx)`**：取消上下文不放 Config；正常 Teardown 传 `Background`（勿用已取消的 runCtx 跳过 ICS 清理）。

---

## 历史轮次

架构解耦第 12～25 轮及更早变更摘要**只写** [dev-log.md](dev-log.md)（本文件不堆轮次长文，避免与进度源双份漂移）。
本轮（第 29）：ICS/egress PS 集中、cmd 门面、PreferVPN 组装、死代码收口；见 dev-log 顶部「架构审计第 29 轮」。

---

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
| GET/POST | `/api/v1/security/exempts` | handleSecurityExempts | Session |
| DELETE | `/api/v1/security/exempts/{ip}` | handleSecurityExemptByIP | Session |
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
| **AllowedIPs** | 经**服务端网关/NAT**可达的工控网段（及可选 VPN 子网）；握手时另并入策略 DNS `/32` | `users.allowed_ips` + 默认 NAT CIDR；`MergeDNSIntoAllowedIPs`；握手 `allowed_ips` |
| **local_lans** | 客户端 YAML/GUI **手动**配置的本机后面 LAN；非空才开启 | `client.yaml` → 握手 `local_lans` → 临时表 `client_lan_registry`；客户端 `netstack` via 出口 |
| **托管路由 Managed Routes** | `dest via 客户端`（hub 转 via，via 再出 LAN） | 定义 `peer_routes` + 访问方 `peer_route_members`（`user_id=0`=全部）；握手仅下发**非失效**项 |
| **互访** | 默认可 ping 对方 `vpn_ip/32` 禁止 | `security.allow_all_vpn_peers` / `peer_access`（默认双向）；「应用生效」踢线刷新 |

客户端托盘「本机路由」：本机 TUN（`vpn_subnet`）+ **分流**（AllowedIPs，含 `nat.allowed_lan_cidrs`）+ **对端托管**（`managed_routes`）。「无对端托管」≠ 未装工控路由。

> **流量怎么走、代码落点、与 OpenVPN push/iroute 对照**：见 [traffic-routing.md](traffic-routing.md)。

**失效**：via 离线，或注册表无匹配 `dest` → UI 标失效；握手跳过，不装客户端路由。注册表 alone **不转发**，须管理员从注册表创建托管路由。

出站 `RouteOutbound`：仅 `dst==vpn_ip` 或托管 via 索引；**禁止**用会话 AllowedIPs（NAT）把流量错送回客户端。入站：横向 → via 匹配（优先于 writeTUN）→ 否则 writeTUN（网关 NAT）。入站越权目的：`netutil.VPNIPOrInNets` + `safeutil.AllowEvery` 限频；客户端上送同公式。

查功能：公式 → `netutil.VPNIPOrInNets` / `IsTUNNoise*`（含 `IsTUNNoiseSource`）；限频 → `safeutil.AllowEvery`；调用 → `clientapp/runtime_tun.go`、`sessionmgr/route_inbound.go`；DNS 并入 → `MergeDNSIntoAllowedIPs`（握手 `server_handshake_session.go`）。

**客户端冷启动时序（家庭版 via / ICS）**：

```text
Handshake(+ local_lans) → TUN open → defer_routes → ApplyDNS（ICS 前）
  → via Setup：egress 快照 → HasICSResidue?
       有 137 → reuse_live（Go iphlp Prefer，秒级）
       无 137 → Force Restart SharedAccess → EnableSharing（PS）→ Go iphlp Prefer（保留 /24）
  → 全量装分流路由 →（viaDidSetup 时 Fast scrub）→ StateConnected
HardRestart → StopKeepICS（ICSPreserve）→ 同上 reuse_live（勿 DisableICSPair）
Logout → Stop（ICSDisable）→ DisableICSSession
```

- **无** post-auth `lan_registry_sync`（仅握手上报注册表）。
- 慢路径文件：`winnet/egress_windows.go`、`ics_egress_windows.go`、`ics_enable_windows.go`、`ics_lifecycle.go`、`ps_snippets.go`、`ps_log.go`、`ics_probe.go`、`default_route_windows.go`、`prefer_vpn_light_windows.go`、`route_ops_windows.go`、`hard_restart.go`。
- PreferVPN：**主路径 iphlp**（`PreferVPNAfterSoftIPReplace`）；PS 仅无 ifIndex / 回退；**禁止** `ics_prefix_fix`（保留 ICS 扩成的 `/24`）。
- **在线仅改 VPN IP**：`vpn_ip_replace_inplace`（KeepICS Replace + iphlp SkipAsSource；routes=keep 若 gw/allowed 未变）。

**客户端断线重连与策略应用（差分）**：

| 路径 | 行为 | 代码 |
|------|------|------|
| 临时断线（自动重连） | **保留** TUN / 分流路由 / via·ICS / DNS；若开杀开关则 Enable | `protectForReconnect` |
| 握手后策略 | 比对内容指纹：路由集合差分增删；via 指纹（lans\|subnet）未变则跳过 ICS；完全一致则 `policy_apply mode=noop`；**预判将跑 ICS 时推迟装路由，Setup 后再装一次**；变 VPN IP 软换 | `runtime.applyPolicy`、`policy_diff.go`、`via_exit.go` |
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
// CLI 交互拨号
clientapp.RunCLI(context.Background(), cfg, creds, clientapp.DefaultInteractiveRunOptions())
// GUI 登录（无 Start，由 StartAndWaitFirstAuth 首连）
eng, _ := clientapp.PrepareEngine(cfg, creds, clientapp.DefaultGUIRunOptions(nil))
clientapp.StartAndWaitFirstAuth(ctx, eng)
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

