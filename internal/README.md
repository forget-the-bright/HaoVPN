# internal/ 包索引

> **改代码去哪**：按功能查下表。完整 CODEMAP、分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)（权威单一来源）。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 | `api/handler_routes.go` |
| 健康 / 审计 / Dashboard / 备份 / 日志 | `api/handler_ops.go`；Dashboard 字段 `health/dashboard.go` |
| 探针事件 / 封禁 API / WebUI | `api/handler_security.go`；逻辑 `probedefense/guard.go`；页 `web/templates/security_probe.html` |
| 托管路由 / 互访 / 应用生效 / 注册表 | `api/handler_peers.go`；`GET /api/v1/lan-registry`；页 `/peers` → `web/templates/peer_routes.html`；表 `peer_routes`+`peer_route_members`/`peer_access`/`client_lan_registry`；`persist/peer_store.go`、`lan_registry.go` |
| 握手策略合并（AllowedIPs∪有效托管 dest；失效跳过；/32 去冗余） | `vpnaccount/peer_policy.go` → `ResolveClientPolicy`；会话 `sessionmgr` ViaRoutes/PeerAccess |
| 客户端 local_lans / via 出口 | `config/client.go`（`local_lans`）；握手上报 `tunnel/handshake.go`+`server_handler.go`；出口 `clientapp/via_exit.go`（复用 `netstack.Stack`）；GUI `clientgui/login.go` |
| 服务端 NAT（工控） | `serverapp/engine_boot.go` + `netstack.Stack`；配置 `nat.allowed_lan_cidrs` |
| 管理 API 多地址监听 | `api/handler_listen.go`（`StartAllListeners`） |
| API 标准错误 / 成功 / 分页信封 / since / 方法守卫 | `api/httputil.go` → `writeOK`/`writeInternalError`/`requireMethod`/`parseFormOrError`/`decodeJSONOrForm` |
| 用户 CRUD / 删除账号 / 末管理员 | `api/users_crud.go` → `vpnaccount.DeleteAccount`（`ErrLastAdmin`）；禁用调 `LogoutAllForUser` |
| VPN 策略 PATCH | `api/users_vpn.go` → `vpnaccount.ApplyVPNPatch` |
| 启禁账号 | `api/users_crud.go` → `vpnaccount.SetAccountEnabled` |
| 管理员重置用户密码 | `api/users_vpn.go` → `auth.ResetPasswordByAdmin` + `LogoutAllForUser` |
| 登录 / Session / CSRF / 自改密 / 注销仅 POST | `api/auth_handlers.go`；`auth/login.go`、`password_ops.go`、`session.go`、`username.go` |
| 登录/握手哨兵错误（含账号已在线） | `auth/errors.go`；客户端 `clientapp/fatal_auth.go` |
| CIDR/LAN/列表工具 | `netutil`：`ValidLANCIDRs`、`NormalizeCIDRList`、`AppendCIDRUnique`、`ValidateAdvertisedLAN` |
| 桌面 GUI / 托盘 / 日志面板 / eng 锁 | `clientgui/`（`engine_stop.go`；`log.go` UI 默认最近 300 行） |
| Web/隧道分表锁定 | `auth/lockout.go` |
| 隧道密码校验 | `auth/tunnel_login.go`（`VerifyTunnelLogin`） |
| 探针 Accept/特征/自动封 | `probedefense/guard.go`；超时忽略 `ignorable.go`；中文 `labels.go` |
| 挂载 Probe（封禁始终生效） | `serverapp/engine_boot.go`（`probeGuard != nil` 即挂） |
| 握手 / 策略下发 / 明文钥策略 | `tunnel/server_handler.go`、`handshake.go`、`source_ip.go` |
| 客户端拨号 / 重连 / 致命鉴权 | `clientapp/engine_*.go`、`fatal_auth.go`（account_online 有限重试）；临时断线 `protectForReconnect` 保留数据面 |
| 客户端策略差分 / ICS 跳过 | `clientapp/policy_diff.go`、`runtime.go`（`applyPolicy`）、`via_exit.go`（via 指纹） |
| 客户端凭据解析 | `clientapp/credentials.go`（YAML 用户名可配服务库密码） |
| 远端地址拆分 | `netutil/hostport.go`（`SplitRemoteAddr`） |
| CIDR/LAN/广播工具 | `netutil`：`NormalizeCIDROrHost`、`ValidateAdvertisedLAN`、`IsLimitedBroadcast` |
| 秒 → Duration | `timeutil/duration.go`（`Seconds`） |
| 审计中文标签 | `audit/labels.go`；页 `/audit`；对照表 `docs/security-hardening.md` §4.4 |
| 备份/导出（POST+CSRF） | `api/handler_ops.go`、`users_export.go`；前端 `HaoVPN.downloadPost` |
| 安全事件/封禁 SQL | `persist/security_store.go` |
| 分页 limit/offset / `?online=` | `paginate/parse.go`、`clamp.go` |
| 数据保留 / 过期封禁清理 | `maintenance/retention.go` |
| 导出 ZIP/YAML（不含私钥） | `api/export_zip.go`、`users_export.go`；YAML 生成 `config/client_export.go` |
| TUN / 路由 / DNS / via 出口 | `clientapp/runtime.go`、`via_exit.go`、`policy_diff.go`；`netstack/`（与服务端 NAT 共用 Stack） |
| SQLite CRUD / 注册表 / 托管迁移 | `persist/store.go`、`peer_store.go`、`lan_registry.go`、`migrate_peer_routes.go`、`users.go`、`security_store.go`、`query_*.go` |

---

## 按包：主要文件职责（第十二轮）

| 包 | 文件 | 做什么 |
|----|------|--------|
| **auth** | `errors.go` | 哨兵：错密/锁定/须改密/无 VPN 等 |
| | `lockout.go` | `webLockouts` / `tunnelLockouts` |
| | `password_ops.go` | 自改密须旧密码、`UserActiveForSession` |
| | `session.go` | `LogoutAllForUser`、CSRF 常量时间、`PruneExpiredSessions` |
| **probedefense** | `guard.go` | Accept 封禁始终查；Enabled 只管自动记录/封 |
| **tunnel** | `server_handler.go` | 握手、OK 失败回滚、明文钥拒绝 |
| | `source_ip.go` | `ErrSourceDenied` + `IPMatchesRules` |
| **clientapp** | `fatal_auth.go` | `errors.Is` + 文案兜底，停止重连 |
| | `policy_diff.go` / `runtime.go` / `via_exit.go` | 路由差分、增量 applyPolicy、via 指纹跳过 ICS |
| | `engine_lifecycle.go` | `protectForReconnect` 保留数据面；`protectThenClearRoutes` 全清 |
| **netutil** | `hostport.go` | `SplitRemoteAddr` |
| **timeutil** | `duration.go` | `Seconds` |
| **persist** | `security_store.go` | `fillIPBlock` 合一扫描 |
| **sessionmgr** | `route.go` / `kick.go` | 发送前确认 Conn；回调锁内拷贝 |
| **api** | `auth_handlers.go` | requireAuth 失败关闭；改密吊销会话 |
| **serverapp** | `engine_boot.go` | 始终挂 Probe |

---

## 分层速览

```
clientapp / clientgui / serverapp
    ├── api ──► vpnaccount / auth / probedefense
    ├── tunnel ──► tun
    ├── transport ← Probe
    ├── netstack ──► platform
    ├── maintenance
    └── persist + sessionmgr
netutil / winnet / fileutil / timeutil / paginate / readmodel / security / config
```

完整包一览表见 [architecture.md § CODEMAP](../docs/architecture.md#internal-包-codemap)。

---

## 第十二轮架构要点（2026-08-29）

- **叶子工具**：`SplitRemoteAddr`、`Seconds`、`fillIPBlock`；源 IP 统一 `IPMatchesRules`。
- **哨兵**：`auth.Err*`；`IsFatalHandshakeError` / 探针签名用 `errors.Is`。
- **P0**：Probe 始终挂载；握手回滚；改密旧密码+吊销 Session；requireAuth 失败关闭。
- **P1**：明文钥默认拒绝；导出不解密；双 lockout；CSRF 常量时间；retention 解耦。

## 第十三轮架构要点（2026-08-29）

- **netutil**：`NormalizeCIDROrHost`、`ValidateAdvertisedLAN`、`IsLimitedBroadcast`、`NormalizeRemoteHost`。
- **ExitLAN**：仅 via 可旁路横向隔离；local_lans RFC1918+≥/16。
- **管理面**：HTTP 超时；备份/导出 POST+CSRF；审计 `labels.go` + enrichment。

## 第十四轮架构要点（2026-08-30）

- **叶子**：`ValidLANCIDRs`/`NormalizeCIDRList`/`AppendCIDRUnique`；clientapp 不依赖 persist/sessionmgr 仅为校验/哨兵。
- **安全**：公开 health 无 recent_errors；末管理员；logout POST；用户名域校验；500 稳定文案。
- **API/GUI**：httputil 辅助；online 分页；eng 与 engOpMu 同锁。

## 第十一轮架构要点（2026-08-28）

- **vpnaccount**：`releaseDynamicIP`；`PlanVPNPatch` 在 `patch.go`；`ErrAccountNotFound` 统一。
- **api**：`users_crud` 复用 `onlineUserSet()`；导出/重置密码 404 语义对齐。
- **授权**：[docs/licensing.md](../docs/licensing.md)。
