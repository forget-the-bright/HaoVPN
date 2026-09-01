# internal/ 包索引

> **改代码去哪（完整 FAQ）**：以 [docs/architecture.md](../docs/architecture.md) 为**权威单一来源**。本文件只保留高频捷径 + 按包文件说明，避免与 CODEMAP 双份漂移。

---

## 高频捷径（详细条目见 architecture FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API / VPN 账号写 | `api/` + `vpnaccount/` |
| 握手 / 拨号错误 | `autherr/`、`dialerr/`、`tunnel/`、`clientapp/fatal_auth.go` |
| Soft / Hard 重连 | Soft：`transport/reconnect.go`；Hard：`clientapp/hard_restart.go`（DNS settle + PollUntil）；GUI：`reconnect_dns.go` + `engine_intent.go` / `engine_op_queue.go` |
| 在线改 VPN IP（同 IP 保 / 变 IP 软换） | 判断：`runtime_policy.go`（`vpn_ip_inplace`）；软换：`ReplaceTUNIPv4KeepICS` + `ApplyPreferVPNSkipAsSource`（`iphlp_skipas_windows.go`）；routes=keep：`routeListsEqual` |
| 分流目的过滤 / 越权 WARN | 公式：`netutil.VPNIPOrInNets`；噪声：`IsTUNNoiseDst`/`IsTUNNoiseForLog`/`IsTUNNoiseSource`；限频：`safeutil.AllowEvery`；DNS：`MergeDNSIntoAllowedIPs`；调用：`runtime_tun` / `route_inbound` |
| Stop / policy abort / ICS | `engine_lifecycle.go`（`Stop`=`ICSDisable` / `StopKeepICS`=`ICSPreserve`）；`ics_lifecycle.go`；via：`CleanupICSResidueContext` |
| HardRestart（保活 ICS） | `hard_restart.go` → `StopKeepICS` → 有 137 则 `reuse_live` |
| GUI Stop 串行 / 意图排队 | `engine_stop.go`（`stopEnginesSerial`）；`engine_op_queue.go`；`login_fail.go`（`safeutil.IsDeadline`） |
| Windows PS / ICS / WinNAT / 出站 / DNS / 孤儿 | `ps_snippets.go`、`ps_log.go`、`ics_probe.go`、`prefer_vpn_light_windows.go`、`ics_*`；门面 `PreferVPNAfterSoftIPReplace` |
| local_lans / replay | 服务端：`sessionmgr/route_inbound.go`（Conn 绑定）、`register.go` + `register_grace.go` + `register_lan.go`、`crypto/wg_crypto.go`；客户端：`engine_connect.go` `tunUploadReady`；注册表握手：`tunnel/server_handler.go` `applyLANRegistry`（旧客户端兼容：`tunnel/lan_registry_sync.go`） |
| ctx 取消 / 可中止轮询 / 截止 | `safeutil.IsCanceled` / `Check` / `IsDeadline` / `PollUntil` |
| TUN 预热 / 服务凭据 / CLI·GUI 启动 | `clientapp.StartWarmupAsync` / `RunCLI` / `PrepareEngine` / `RunServiceLoop`（GUI 禁止 import tun/winnet/credentials/autostart 编排） |
| 叶子工具 | `netutil`（`ValidateLocalLANsList` 硬挡 / `ValidLANCIDRs` 软洗）/ `fileutil` / `timeutil` / `paginate` / `platform` |

---

## 按包：主要文件（现行）

| 包 | 文件 | 做什么 |
|----|------|--------|
| **dialerr** | `errors.go` / `classify.go` | 拨号哨兵（中文 Error）、banner 常量、共用前缀匹配、FatalDial、TLS bad-record |
| **autherr** | `classify.go` | 分类 + code；子串表与 Is* 共用；依赖 dialerr，不依赖 transport |
| **probedefense** | `guard.go`（`OnHandshakeReject`）/ `classify_*.go` | 探针；实现 tunnel.ProbeRecorder；无 ErrSourceDenied re-export |
| **clientapp** | `engine_bootstrap.go` / `bootstrap.go` / `connect_failure.go` / `connect_warn.go` / `dial_errors.go` / `autostart_facade.go` / `hooks.go` / `engine_*.go` / `hard_restart.go` / `warmup.go` / `via_exit.go` | 启动契约；HardRestart；via/ICS（经 netstack 门面） |
| **clientgui** | `run.go` / `tray_*.go` / `service_takeover.go` / `engine_intent.go` / `engine_stop.go` / `reconnect_dns.go` | 托盘/重连/服务接管；`prepareGUIEngine`；禁 autostart 直接编排 |
| **sessionmgr** | `register*.go` / `register_grace.go` / `register_lan.go` / `route_inbound.go` / `route_policy.go` / `stats.go` | 在线会话、via 旁路、横向隔离 |
| **persist** | `store.go` / `peer_*.go` / `lan_registry.go` | SQLite 与 peer/LAN 表 |
| **auth** | `session.go` / `tunnel_login.go` / `lockout.go` | Web/隧道鉴权与会话 |
| **crypto** | `wg_crypto.go` | 隧道加解密与防重放 commit |
| **tun** | `tun_*.go` / `wintundll/` | TUN 设备；GUI 经 `StartWarmupAsync` |
| **transport** | `transport.go`（`noteSendQueueFull` 限频）/ `reconnect.go` / `probe_banner.go` | Conn 队列背压可观测；重连 Done/ExpBackoff；banner |
| **tunnel** | `server_handler.go` / `server_handshake_*.go` / `handshake*.go` | 握手编排文件簇；源 IP 直接 netutil |
| **netutil** | `ipmatch.go`（`VPNIPOrInNets`/`MergeDNSIntoAllowedIPs`）/ `addr.go`（`IsTUNNoise*`）/ `gateway.go`（`ResolveGateway` 两参） | 分流目的公式；DNS 并入；TUN 噪声；网关 |
| **safeutil** | `throttle.go`（`AllowEvery`）/ `poll.go` / `context.go`（`IsCanceled`/`IsDeadline`/`Check`）/ `goroutine.go` / `retry.go` | 日志限频；可中止轮询；ctx；GoSafe；退避 |
| **winnet** | `ipv4_replace.go` / `iphlp_*` / `ics_probe.go` / `ps_log.go` / `prefer_vpn_light_windows.go` / `ps_snippets.go` / `ics_*` | 配 IP；137 探测；ICS PS 日志；Prefer iphlp；PS 模板 |
| **netstack** | `ics_lifecycle.go` / `ics_enable_` / `ics_egress_` / `winnet_facade.go` / `dns_windows.go` | ICS Disable/Preserve；Enable/reuse；瘦门面 |
| **api** | `auth_handlers.go` / `httputil.go` / `handler_peer_*.go` | Cookie helpers；`writeDomainError`；HTTP 薄层 |
| **vpnaccount** | `peer_write.go` / `peer_apply.go` / `provision.go` | peer 写+脏标；开户/策略 |
| **serverapp** | `engine.go` / `engine_boot.go` / `boot_persist.go` / `boot_ippool.go` / `boot_session.go` / `boot_tun.go` / `boot_tunnel.go` / `boot_api.go` / `engine_shutdown.go` | 启动分阶段；peerDirty 重启 WARN |
| **maintenance** | `retention.go` | 审计/日志/封禁保留后台 |
| **audit** | `audit.go` / `labels.go` / `public_bind.go` / `tun_listen.go` | 管理审计文案 |
| **health** | `health.go` / `dashboard.go` | 启动自检；Dashboard 字段 |
| **logstore** | `logstore.go` | 结构化历史日志 |
| **ippool** | `pool.go` | VPN IP 池 |
| **credentials** | `windows.go` | Windows 服务 DPAPI 凭据 |
| **platform** | `elevate_*.go` / `cmd_*.go` / `cmderr.go` | UAC；无窗口子进程 |
| **singleinstance** | `lock.go` / `coord.go` | 客户端 TCP 单实例 |
| **paginate** | `parse.go` / `clamp.go` | API 分页与 bool 查询 |
| **timeutil** | `sqlite.go` / `rfc3339.go` / `duration.go` / `timezone.go` | SQLite UTC；展示时区 |
| **brand** | `brand.go` | Wintun 池名等产品常量 |
| **version** | `version.go` | 构建版本（读根目录 VERSION） |
| **fileutil** | `fs.go` / `mkdir.go` / `perm_*.go` | EnsureDir、AbsPair、ACL |
| **security** | `tls_policy.go` | CSP `script-src`/`style-src` 均 `'self'` |
| **config** | `client.go` / `tun_name.go` | YAML；`ValidateTunName` |
| **logger** | `logger.go` / `redact.go` | 分级日志；敏感字段脱敏 |
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
| `engine_bootstrap.go` / `bootstrap.go` | PrepareEngine / RunCLI / RunServiceLoop |
| `hard_restart.go` / `warmup.go` | HardRestart / waitDNSReady；StartWarmupAsync |
| `dial_errors.go` / `fatal_auth.go` / `connect_failure.go` / `connect_warn.go` | 拨号 UX、首连失败、已连告警 |
| `autostart_facade.go` | 登录/服务自启（GUI 门面） |
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
