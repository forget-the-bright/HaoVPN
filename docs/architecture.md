# HaoVPN 架构与 CODEMAP

本文是重构后的**包导航单一来源**：分层、依赖规则、改代码去哪找。

> 架构解耦第十一轮（2026-08-28）：vpnaccount `releaseDynamicIP`、PlanVPNPatch 归 patch.go、ErrAccountNotFound 对齐、users 列表 `onlineUserSet`、审计补测。第十轮摘要见 [dev-log.md](dev-log.md)。

---

## 分层

```
cmd/client, cmd/client-gui, cmd/server   # 入口：flag、单实例、提权（GUI UI 在 clientgui）
        │
        ▼
clientapp / clientgui / serverapp        # 应用编排与桌面 UI
        │
        ├── api ──► vpnaccount           # HTTP 薄层；VPN 写经 ApplyVPNPatch/SetEnabled
        ├── tunnel ──► tun               # 握手协议；ServerHandler 持有 tun.Device
        ├── transport                    # TLS-TCP 帧
        ├── netstack ──► platform        # 路由/DNS/杀开关；无窗口子进程
        ├── maintenance                  # 数据保留后台（serverapp 启动，与 api 解耦）
        └── persist, auth, sessionmgr    # 存储与会话
        │
        ▼
netutil, winnet, paginate, security, config, fileutil, timeutil, readmodel  # 叶子工具
```

---

## 改代码去哪（FAQ）

| 需求 | 目录 / 文件 |
|------|-------------|
| 新增管理 API | `api/users_crud.go` + `handler_routes.go`；业务 `vpnaccount/` |
| API JSON 错误 / `?since=` / 成功信封 | `api/httputil.go`（`writeAPIError`、`writeOK`、`writePage`、`parseSinceQuery`→`timeutil`） |
| 管理 API 监听绑定 | `api/handler_listen.go`（`StartAllListeners`、`FormatBoundAddrs`） |
| CSRF Token | `api/auth_handlers.go`（`handleCSRF`） |
| VPN 策略 PATCH / 启禁 | `vpnaccount/patch.go`（`ApplyVPNPatch`）、`enable.go`（`SetAccountEnabled`） |
| 删 VPN 账号（踢线+释 IP） | `vpnaccount/delete.go` |
| 改密 / 须改密检查 | `auth/password_ops.go`（`ChangePassword`、`MustChangePassword`、`ResetPasswordByAdmin`） |
| 分页 limit/offset / bool 查询 | `paginate/parse.go`（`ParseLimitOffset`、`ParseBoolQuery`）、`clamp.go` |
| 默认 IP 租约秒数 | `persist/constants.go`（`DefaultIPLeaseSec`，与 schema 同源） |
| 客户端 IP / 反代 XFF | `api/httputil.go`（`resolveClientIP`、`api.trusted_proxy_cidrs`） |
| 日志脱敏 | `logger/redact.go`；`security.Redact` 委托 |
| 密码强度 | `auth/password.go`（`ValidatePasswordStrength`） |
| 审计/连接事件/日志保留 | `maintenance/retention.go`；默认天数 `config.DefaultRetentionDays` |
| 监控 online/accounts/events | `api/monitor_handler.go`；JOIN `persist/query_monitor.go` |
| 用户/审计/事件列表 SQL | `persist/query_users.go`、`query_audit.go`、`query_events.go` |
| 改握手/策略下发 | `tunnel/handshake.go`, `server_handler.go` |
| 客户端拨号/重连 | `clientapp/engine_lifecycle.go`, `engine_connect.go`, `runtime.go` |
| 桌面 GUI（Fyne） | `internal/clientgui/`（入口 `cmd/client-gui` 仅 flag/UAC/主题） |
| 服务端启动流程 | `serverapp/engine.go`、`engine_boot.go`、`engine_shutdown.go` |
| YAML 默认值/校验 | `config/client.go`、`server.go` |
| 默认 TLS 证书路径 | `config/paths.go`（`DefaultServerCertPath`、`ResolveServerCertPath`） |
| 导出客户端 YAML | `config/client_export.go`；ZIP 在 `api/export_zip.go`；HTTP 附件 `writeAttachment` |
| GUI 写回 client.yaml | `config/client_yaml_patch.go`（`SaveClient`）；Node 原语 `yaml_node.go` |
| CIDR/地址/IPv4 工具 | `internal/netutil/`（`HostFromAddr` 供 api clientIP 与隧道共用） |
| SQLite / RFC3339 时间 | `timeutil/sqlite.go`、`timeutil/rfc3339.go` |
| 敏感文件原子写 / exe 目录 | `fileutil/WriteFileAtomic`、`ExecutableDir` |
| Web/API 读模型 | `readmodel/`（含 `audit.go` AuditLogView） |
| Dashboard 字段 | `health/dashboard.go`（`DashboardMap`） |
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
| **clientapp** | CLI/GUI 共用拨号引擎 | `engine_state.go`, `engine_lifecycle.go`, `engine_connect.go`, `runtime.go`, `credentials.go` | config, transport, tunnel, netstack |
| **clientgui** | Fyne 桌面 UI | `run.go`, `login.go`, `app.go`, `tray.go`, `notice.go` | clientapp, config, singleinstance |
| **serverapp** | 服务端启动编排 | `engine.go`, `engine_boot.go`, `engine_shutdown.go` | api, tunnel, tun, netstack, vpnaccount, maintenance |
| **api** | HTTP 管理 API + WebUI | `handler.go`, `handler_routes.go`, `handler_ops.go`, `handler_listen.go`, `auth_handlers.go`, `users_*.go`, `monitor_handler.go`, `httputil.go`, `export_zip.go` | auth, vpnaccount, persist, readmodel, config, netutil |
| **readmodel** | Web/API 读模型 DTO | `types.go`, `monitor.go`, `audit.go` | timeutil |
| **paginate** | 分页 / bool 查询 | `clamp.go`, `parse.go`（含 `ParseLimitOffset`） | — |
| **maintenance** | 数据保留后台 | `retention.go`（`GoSafe` 启动） | persist, logstore, config, safeutil |
| **fileutil** | 父目录 / 原子写 / exe 目录 | `mkdir.go`, `atomic.go`, `exe.go` | — |
| **timeutil** | SQLite UTC + RFC3339 | `sqlite.go`, `rfc3339.go` | — |
| **vpnaccount** | IP 模式、开户、PATCH、启禁、删号 | `service.go`, `provision.go`, `patch.go`, `enable.go`, `delete.go` | ippool, persist, netutil |
| **tunnel** | 握手协议 | `handshake.go`, `client_handshake.go`, `server_handler.go` | transport, crypto, netutil, **tun** |
| **transport** | TLS-TCP 帧、重连 | `transport.go`, `frame.go`, `reconnect.go` | netutil, config |
| **sessionmgr** | 会话与报文路由 | `manager.go`, `register.go`, `kick.go`, `route.go`, `stats.go` | crypto, netutil, persist, config |
| **probedefense** | 公网探针识别/落库/封禁 | `guard.go`, `labels.go`, `ignorable.go`, `config_from.go` | persist, netutil, config, logger |

| **netstack** | 路由/DNS/杀开关/NAT | `route_*.go`, `dns_*.go` | winnet, netutil, **platform** |
| **tun** | TUN 抽象 | `tun.go`, `tun_windows.go` | winnet, **wintundll**, fileutil |
| **wintundll** | 嵌入/释放 wintun.dll | `ensure.go` | fileutil |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` | platform |
| **netutil** | CIDR/地址/监听/MTU | `cidr.go`, `addr.go`, `gateway.go`, `listen.go`, `constants.go` | — |
| **config** | YAML 加载/导出/默认值 | `config.go`, `client_export.go`, `client_yaml_patch.go`, `yaml_node.go`, `paths.go`, `retention.go` | netutil, fileutil, brand |
| **security** | TLS、密钥加密、绑定自检 | `tls_client.go`, `datakey.go`, `keyenc.go` | netutil, fileutil |
| **persist** | SQLite | `store.go`, `constants.go`, `users.go`, `query_*.go`, `security_store.go`, `session_store.go` | paginate, readmodel, timeutil |
| **auth** | Web Session + 隧道密码校验 | `service.go`, `login.go`, `tunnel_login.go`, `session.go`, `lockout.go`, `password.go`, `password_ops.go` | persist |
| **ippool** | VPN IP 池 | `pool.go` | — |
| **health** | 启动自检 + Dashboard | `health.go`, `dashboard.go` | config, persist |
| **logstore** | 结构化历史日志库 | `logstore.go` | paginate, timeutil |
| **audit** | 管理审计 | `audit.go` | persist |
| **logger** | 分级日志 | `logger.go` | — |
| **safeutil** | GoSafe、Ticker、Shutdown | `goroutine.go`, `ticker.go` | — |
| **crypto** | 隧道加解密 | `wg_crypto.go` | — |
| **credentials** | Windows DPAPI 凭据 | `windows.go` | fileutil |
| **platform** | UAC、无窗口子进程、错误包装 | `elevate_windows.go`, `cmderr.go` | — |
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

---

## HTTP API 路由表

注册于 `internal/api/handler_routes.go` `routes()`；写操作须 Session + CSRF。

| 方法 | 路径 | Handler | 鉴权 |
|------|------|---------|------|
| POST | `/api/v1/login` | handleLogin | 公开 |
| GET | `/api/v1/health` | handleHealth | 公开 |
| GET | `/api/v1/system/info` | handleSystemInfo | 公开 |
| POST | `/api/v1/logout` | handleLogout | Session |
| POST | `/api/v1/password` | handleChangePassword | Session |
| GET | `/api/v1/csrf` | handleCSRF | Session |
| GET/POST | `/api/v1/users` | handleUsers | Session |
| * | `/api/v1/users/{id}/…` | handleUserByID | Session |
| GET | `/api/v1/audit` | handleAudit | Session |
| GET | `/api/v1/dashboard` | handleDashboard | Session |
| GET | `/api/v1/backup` | handleBackup | Session |
| GET | `/api/v1/logs` | handleLogs | Session |
| GET | `/api/v1/monitor/online` | handleMonitorOnline | Session |
| GET | `/api/v1/monitor/accounts` | handleMonitorAccounts | Session |
| GET | `/api/v1/monitor/events` | handleMonitorEvents | Session |
| GET | `/api/v1/security/events` | handleSecurityEvents | Session |
| GET/POST | `/api/v1/security/blocks` | handleSecurityBlocks | Session |
| DELETE | `/api/v1/security/blocks/{ip}` | handleSecurityBlockByIP | Session |

**WebUI**：`/`, `/users`, `/connections`, `/audit`, `/security`（探针）、`/tools`；`/peers` → `/users`；`/login` 公开。

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
