# internal/ 包索引

> **改代码去哪**：按功能查下表。完整 CODEMAP、分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)（权威单一来源）。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 | `api/handler_routes.go` |
| 健康 / 审计 / Dashboard / 备份 / 日志 | `api/handler_ops.go`（公开 health 仅 ok+uptime；历史日志 items 再脱敏）；Dashboard 字段 `health/dashboard.go` |
| 托管路由 / 互访 / LAN 注册 / HTTP 应用生效 | `api/handler_peer_routes.go`、`handler_peer_access.go`、`handler_lan_registry.go`、`handler_peers_apply.go`、`handler_peers_dirty.go`；DTO `readmodel/peers.go` |
| peer dirty / 应用生效（领域） | `vpnaccount/peer_apply.go`（`PeerPolicyApplier`）；重启 WARN `serverapp/boot_api.go` |
| Session Cookie 写入/清除 / 滑动续期 | `api/auth_handlers.go`：`setSessionCookie` / `clearSessionCookie`（Secure/SameSite 对齐）；Touch 重发 |
| API JSON 成功 / pending_apply / items / JSON 体上限 / 方法守卫 | `api/httputil.go` → `writeOK*`/`writePendingApply`/`writeItems*`/`decodeJSONBody`（1MiB）/`requireMethod` |
| WebUI CSP / 页面脚本 | CSP `security/tls_policy.go`（`script-src 'self'`；style 仍 unsafe-inline）；`web/static/*.js` |
| 短时重试（Listen 等） | `safeutil/retry.go`（`RetryN`） |
| AbsPair / EnsureDir / ACL / 世界可读 | `fileutil/fs.go`、`mkdir.go`、`perm_*.go`（`RestrictToAdminsOnly`、`CheckWorldReadable`） |
| 广告 LAN 禁 VPN 池重叠 | `netutil.ValidateAdvertisedLANNotForbidden`；握手 `tunnel/server_handler.go` |
| GUI 开机自启（计划任务/服务） | `autostart/`（Win SCM+计划任务；Linux XDG/systemd；macOS LaunchAgent/Daemon；`gen.go`；`paths_unix.go` AbsPair） |
| 探针事件 / 封禁 API / WebUI | `api/handler_security.go`（POST `duration_sec`）；逻辑 `probedefense/guard.go` `ManualBan(ip, reason, durationSec)`；页 `web/templates/security_probe.html` + `static/security_probe.js` |
| 握手策略合并 | `vpnaccount/peer_policy.go` → `ResolveClientPolicy`；会话 `sessionmgr` ViaRoutes/PeerAccess |
| 客户端 local_lans / via 出口 | `config/client.go`；握手 `tunnel/`；出口 `clientapp/via_exit.go`；GUI `clientgui/login.go` |
| 服务端 NAT（工控） | `serverapp/boot_tun.go` + `netstack.Stack`；配置 `nat.allowed_lan_cidrs` |
| 用户 CRUD / 删号 / 末管理员 | `api/users_crud.go` → `vpnaccount.DeleteAccount` |
| VPN 策略 PATCH / 启禁 | `api/users_vpn.go` → `vpnaccount.ApplyVPNPatch` / `SetAccountEnabled` |
| 登录 / Session / CSRF / 自改密 | `api/auth_handlers.go`；`auth/`（密码 ≤72） |
| CIDR/LAN/列表工具 | `netutil`（含 `ValidateAdvertisedLANNotForbidden`、`StringSlicesEqualTrimmed`） |
| 桌面 GUI / 托盘 / eng 锁 / 管理员门禁 | `clientgui/`（`admin.go` `requireAdmin`；`tray_config.go`；`engine_stop.go`） |
| 布尔查询/表单 | `paginate.ParseBoolQuery`（api 表单与 `persist/settings.go` 共用） |
| TUN / 路由 / DNS / via | `clientapp/runtime.go`、`runtime_policy.go`、`runtime_routes.go`、`runtime_tun.go`、`via_exit.go`；`netstack/` |
| SQLite / 托管 / 注册表 | `persist/store.go`、`peer_*.go`、`lan_registry.go`、`users.go`、`query_*.go` |
| 会话路由 / 横向隔离 | `sessionmgr/route.go`、`route_inbound.go`、`route_lookup.go`、`route_policy.go` |
| TLS 帧 / 重连 | `transport/transport.go`、`config.go`、`conn_loops.go`、`server.go`、`mtu.go` |
| 服务端启动阶段 | `serverapp/engine_boot.go` + `boot_*.go` |
| Windows UAC 提权（含空格路径） | `platform/elevate_windows.go`（`EscapeArg`） |
| 日志脱敏 | `logger/redact.go` |

---

## 按包：主要文件（第十七轮）

| 包 | 文件 | 做什么 |
|----|------|--------|
| **vpnaccount** | `peer_apply.go` | `PeerPolicyApplier`：脏标记与应用生效（出 api） |
| **api** | `auth_handlers.go` / `httputil.go` / `handler_peers_*.go` | Cookie helpers；decodeJSONBody；HTTP 薄层委托 Applier |
| **serverapp** | `boot_persist.go` … `boot_api.go` | 启动分阶段；peerDirty 重启 WARN；Listen 用 RetryN |
| **transport** | `transport.go` 等 | Conn.Close 锁拷贝 onClose |
| **sessionmgr** | `route_*.go` | viaIndex 重建稳定排序 |
| **persist** | `peer_access.go` 等 | 互访须已存在 VPN 用户 |
| **fileutil** | `fs.go` / `mkdir.go` / `perm_*.go` | EnsureDir、AbsPair、RestrictToAdminsOnly、CheckWorldReadable |
| **safeutil** | `retry.go` | `RetryN` |
| **netutil** | `slices.go` | `StringSlicesEqualTrimmed` |
| **security** | `tls_policy.go` | CSP `script-src 'self'` |
| **logger** | `redact.go` | Authorization / session= 脱敏 |
| **credentials** | `windows.go` | 写后 RestrictToAdminsOnly |
| **platform** | `elevate_windows.go` | UAC EscapeArg |
| **readmodel** | `peers.go` | Peer 视图 DTO |
| **autostart** | `logon_*.go` / `service_*.go` / `gen.go` | 跨平台自启 |

---

## 分层速览

```
clientapp / clientgui / serverapp
    ├── api ──► vpnaccount（含 PeerPolicyApplier）/ auth / probedefense
    ├── tunnel ──► tun
    ├── transport ← Probe
    ├── netstack ──► platform
    ├── maintenance
    └── persist + sessionmgr
netutil / winnet / fileutil / timeutil / paginate / readmodel / security / config / safeutil
```

完整包一览见 [architecture.md § CODEMAP](../docs/architecture.md#internal-包-codemap)。

> 架构轮次变更摘要只写 [docs/dev-log.md](../docs/dev-log.md)，本文件不重复堆「第 N 轮」。
