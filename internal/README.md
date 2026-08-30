# internal/ 包索引

> **改代码去哪**：按功能查下表。完整 CODEMAP、分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)（权威单一来源）。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 | `api/handler_routes.go` |
| 健康 / 审计 / Dashboard / 备份 / 日志 | `api/handler_ops.go`（公开 health 仅 ok+uptime）；Dashboard 字段 `health/dashboard.go` |
| 托管路由 / 互访 / LAN 注册 / 应用生效 | `api/handler_peer_routes.go`、`handler_peer_access.go`、`handler_lan_registry.go`、`handler_peers_apply.go`、`handler_peers_dirty.go`；DTO `readmodel/peers.go` |
| API JSON 成功 / pending_apply / items / 表单 int64 / 方法守卫 | `api/httputil.go` → `writeOK`/`writeOKWith`/`writePendingApply`/`writeItems`/`writeItemsTotal`/`parseFormInt64`/`parseQueryInt64`/`requireMethod` |
| 广告 LAN 禁 VPN 池重叠 | `netutil.ValidateAdvertisedLANNotForbidden`；握手 `tunnel/server_handler.go` |
| GUI 开机自启（计划任务/服务） | `autostart/`（Win SCM+计划任务；Linux XDG/systemd；macOS LaunchAgent/Daemon；`gen.go`；`paths_unix.go` AbsPair） |
| 探针事件 / 封禁 API / WebUI | `api/handler_security.go`；逻辑 `probedefense/guard.go`；页 `web/templates/security_probe.html` |
| 握手策略合并 | `vpnaccount/peer_policy.go` → `ResolveClientPolicy`；会话 `sessionmgr` ViaRoutes/PeerAccess |
| 客户端 local_lans / via 出口 | `config/client.go`；握手 `tunnel/`；出口 `clientapp/via_exit.go`；GUI `clientgui/login.go` |
| 服务端 NAT（工控） | `serverapp/boot_tun.go` + `netstack.Stack`；配置 `nat.allowed_lan_cidrs` |
| 用户 CRUD / 删号 / 末管理员 | `api/users_crud.go` → `vpnaccount.DeleteAccount` |
| VPN 策略 PATCH / 启禁 | `api/users_vpn.go` → `vpnaccount.ApplyVPNPatch` / `SetAccountEnabled` |
| 登录 / Session / CSRF / 自改密 | `api/auth_handlers.go`；`auth/` |
| CIDR/LAN/列表工具 | `netutil`（含 `ValidateAdvertisedLANNotForbidden`、`ValidLANCIDRs`） |
| 桌面 GUI / 托盘 / eng 锁 / 管理员门禁 | `clientgui/`（`admin.go` `requireAdmin`；`tray_config.go`；`engine_stop.go`） |
| 文件 Exists / AbsPair / 世界可读检测 | `fileutil/fs.go` |
| 布尔查询/表单 | `paginate.ParseBoolQuery`（api 表单与 `persist/settings.go` 共用） |
| TUN / 路由 / DNS / via | `clientapp/runtime.go`、`runtime_policy.go`、`runtime_routes.go`、`runtime_tun.go`、`via_exit.go`；`netstack/` |
| SQLite / 托管 / 注册表 | `persist/store.go`、`peer_*.go`、`lan_registry.go`、`users.go`、`query_*.go` |
| 会话路由 / 横向隔离 | `sessionmgr/route.go`、`route_inbound.go`、`route_lookup.go`、`route_policy.go` |
| TLS 帧 / 重连 | `transport/transport.go`、`config.go`、`conn_loops.go`、`server.go`、`mtu.go` |
| 服务端启动阶段 | `serverapp/engine_boot.go` + `boot_*.go` |

---

## 按包：主要文件（第十六轮）

| 包 | 文件 | 做什么 |
|----|------|--------|
| **transport** | `config.go` / `transport.go` / `conn_loops.go` / `server.go` / `mtu.go` | 配置、Conn API、读写心跳循环、TLS Server、MTU 探测 |
| **persist** | `peer_types.go` / `peer_access.go` / `peer_routes.go` / `peer_route_normalize.go` / `lan_registry.go` | 互访、托管路由、成员规范化、LAN 注册（含 host_id 截断） |
| **sessionmgr** | `route.go` / `route_inbound.go` / `route_lookup.go` / `route_policy.go` / `stats.go` | 出站/入站/查找/策略；ExitLAN 仅 via |
| **clientapp** | `runtime.go` / `runtime_policy.go` / `runtime_routes.go` / `runtime_tun.go` / `service_*.go` | 策略应用、路由差分、TUN 上送；SCM 薄封装 |
| **serverapp** | `boot_persist.go` … `boot_api.go` | 启动分阶段 |
| **api** | `httputil.go` / `handler_peer_*.go` / `doc.go` | HTTP 辅助；托管编排；边界说明 |
| **readmodel** | `peers.go` | PeerRoute/Access/LANRegistry 视图 DTO |
| **fileutil** | `fs.go` | Exists、AbsPair、CheckWorldReadable |
| **autostart** | `paths_unix.go` / `gen.go` | AbsPair；systemd ExecStart 引号 |
| **clientgui** | `admin.go` | `requireAdmin` OS 提权门禁 |

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

完整包一览见 [architecture.md § CODEMAP](../docs/architecture.md#internal-包-codemap)。

---

## 第十五～十六轮摘要

- **十五**：autostart SCM 唯一写路径；跨平台自启；peer handlers 拆分；公开 health 仅 ok+uptime。
- **十六**：dirty 旧∪新 / apply TOCTOU；ExitLAN≠VPN 池；叶子 helper 统一；胖文件同包拆分；peer DTO→readmodel；systemd 空格引号。

更早轮次见 [docs/dev-log.md](../docs/dev-log.md)。
