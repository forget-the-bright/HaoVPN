# HaoVPN 架构与 CODEMAP

本文是重构后的**包导航单一来源**：分层、依赖规则、改代码去哪找。

---

## 分层

```
cmd/client, cmd/client-gui, cmd/server   # 入口：flag、单实例、提权（GUI 见 client-gui）
        │
        ▼
clientapp / serverapp                    # 应用编排
        │
        ├── api ──► vpnaccount           # HTTP 管理 vs 账号/IP 领域（api 不 import ippool）
        ├── tunnel ──► tun               # 握手协议；ServerHandler 持有 tun.Device
        ├── transport                    # TLS-TCP 帧
        ├── netstack ──► platform        # 路由/DNS/杀开关；无窗口子进程
        ├── maintenance                  # 数据保留后台（serverapp 启动，与 api 解耦）
        └── persist, auth, sessionmgr    # 存储与会话
        │
        ▼
netutil, winnet, paginate, security, config, fileutil, readmodel  # 无业务状态公共能力
```

---

## 改代码去哪（FAQ）

| 需求 | 目录 / 文件 |
|------|-------------|
| 新增管理 API | `internal/api/users.go` + `handler.go` routes；业务 `vpnaccount/` |
| 删 VPN 账号（踢线+释 IP） | `internal/vpnaccount/delete.go`（api 调 `DeleteAccount`） |
| 分页 limit/offset | `internal/paginate/clamp.go`（api、persist、logstore 共用） |
| 审计/连接事件/日志保留 | `internal/maintenance/retention.go` |
| 改握手/策略下发 | `internal/tunnel/handshake.go`, `server_handler.go` |
| 客户端拨号/重连 | `internal/clientapp/engine.go`, `runtime.go` |
| 桌面 GUI（Fyne） | `cmd/client-gui/main.go`, `theme.go` |
| 服务端启动流程 | `internal/serverapp/engine.go` |
| YAML 默认值/校验 | `internal/config/client.go`（ApplyDefaults）、`server.go` |
| CIDR/地址/IPv4 工具 | `internal/netutil/`（addr.go、SplitCIDR、NormalizeIPv4） |
| Web/API 读模型 | `internal/readmodel/types.go`, `monitor.go` |
| 文件父目录创建 | `internal/fileutil/EnsureParentDir` |
| Windows 路由/DNS/杀开关 | `internal/netstack/` + `internal/winnet/` |
| 无窗口 route/netsh 子进程 | `internal/platform/`（netstack 调用） |
| TUN 设备 | `internal/tun/`（tunnel.ServerHandler.TunDev） |
| TLS 客户端/服务端 | `internal/security/tls_client.go`, `cert.go` |
| 传输心跳/重连参数 | `internal/transport/config_from.go` |
| WebUI 静态资源 | `web/embed.go` |
| 包索引（改 X 来哪） | [internal/README.md](../internal/README.md) |

---

## cmd/ 入口

| 目录 | 职责 |
|------|------|
| `cmd/server` | `-c server.yaml` → `serverapp.New(cfg, path).Run()` |
| `cmd/client` | CLI 拨号、Windows `--service`、单实例锁 |
| `cmd/client-gui` | Fyne 桌面 GUI（`main.go` 登录/主窗/托盘，`theme.go` 可读主题；共用 `clientapp.Engine`） |

---

## internal/ 包 CODEMAP

| 包 | 职责 | 关键文件 | 依赖 |
|----|------|----------|------|
| **clientapp** | CLI/GUI 共用拨号引擎 | `engine.go`, `runtime.go` | config, transport, tunnel, netstack |
| **serverapp** | 服务端启动编排 | `engine.go` | api, tunnel, tun, netstack, vpnaccount |
| **api** | HTTP 管理 API + WebUI | `handler.go`, `users.go`, `auth_handlers.go`, `export.go`, `monitor_handler.go`, `logs.go` | auth, vpnaccount, persist, readmodel, paginate |
| **readmodel** | Web/API 读模型 DTO | `types.go`, `monitor.go` | — |
| **paginate** | 分页参数规范化 | `clamp.go` | — |
| **maintenance** | 数据保留后台任务 | `retention.go` | persist, logstore, config |
| **fileutil** | 文件系统小工具 | `mkdir.go` | — |
| **vpnaccount** | IP 模式、开户、删号 | `service.go`, `provision.go`, `delete.go` | ippool, persist, netutil |
| **tunnel** | 握手协议 | `handshake.go`, `server_handler.go`, `source_ip.go` | transport, crypto, netutil, **tun** |
| **transport** | TLS-TCP 帧、重连 | `transport.go`, `frame.go`, `reconnect.go` | netutil, config |
| **sessionmgr** | 会话与报文路由 | `manager.go`, `conn.go` | crypto, netutil（PacketConn 接口） |
| **netstack** | 路由/DNS/杀开关/NAT | `route_*.go`, `dns_*.go` | winnet, netutil, **platform** |
| **tun** | TUN 抽象 | `tun.go`, `tun_windows.go` | winnet, wintundll |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` | — |
| **netutil** | CIDR/地址/监听/MTU | `cidr.go`, `addr.go`, `gateway.go` | — |
| **config** | YAML 加载/默认值 | `config.go`, `client.go`, `client_paths.go` | netutil |
| **security** | TLS、密钥加密、绑定自检 | `tls_client.go`, `keyenc.go`, `tls_policy.go` | netutil |
| **persist** | SQLite | `store.go`, `query_ext.go`, `scan.go`, `jsoncol.go`, `timefmt.go` | paginate, readmodel |
| **auth** | Web 登录 Session | `user.go` | persist |
| **ippool** | VPN IP 池 | `pool.go` | — |
| **health** | 启动自检 | `health.go` | config, persist |
| **logstore** | 结构化历史日志库 | `logstore.go` | — |
| **audit** | 管理审计 | `audit.go` | persist |
| **logger** | 分级日志 | `logger.go` | — |
| **safeutil** | GoSafe、Shutdown | `goroutine.go` | — |
| **crypto** | 隧道加解密 | `wg_crypto.go` | — |
| **credentials** | Windows DPAPI 凭据 | `windows.go` | — |
| **platform** | UAC 提权、无窗口子进程 | `elevate_windows.go`, `cmd_windows.go`, `cmderr.go` | — |
| **singleinstance** | 客户端单实例锁 | `lock.go` | — |
| **brand** | 产品名/路径常量 | `brand.go` | — |
| **version** | 构建版本信息 | `version.go` | — |

每个包均有中文 `doc.go` 说明上下游。

---

## 关键文件索引

> 每包 2～5 个主文件；改 bug 时优先打开这些文件。

| 包 | 主文件 | 职责一句 |
|----|--------|----------|
| **clientapp** | `engine.go` | TLS/握手/重连主循环 |
| | `runtime.go` | TUN、路由、DNS 运行时 |
| | `credentials.go` | yaml → DPAPI → 交互式密码 |
| **serverapp** | `engine.go` | 服务端 Run 八阶段 |
| **api** | `handler.go` | 路由注册、健康/备份/仪表盘 |
| | `users.go` | 账号 CRUD、删号、策略 PATCH、导出 |
| | `auth_handlers.go` | 登录/鉴权中间件 |
| | `httputil.go` | writeJSON、clientIP（分页委托 paginate） |
| | `monitor_handler.go` | 在线监控 API |
| | `logs.go` | 历史日志 tail/分页 |
| | `export.go` | 客户端 YAML/ZIP 导出 |
| **paginate** | `clamp.go` | ClampLimit、ClampOffset |
| **maintenance** | `retention.go` | 审计/连接事件/日志保留清理 |
| **readmodel** | `types.go`, `monitor.go` | UserListItem、MonitorRowToItem 等 DTO |
| **fileutil** | `mkdir.go` | EnsureParentDir |
| **vpnaccount** | `service.go` | IP 模式、握手分配 |
| | `provision.go` | Web 开户、IP 池 |
| | `delete.go` | DeleteAccount：踢线 + 释 IP + 删行 |
| **tunnel** | `handshake.go` | 握手 JSON |
| | `server_handler.go` | 服务端握手（TunDev tun.Device） |
| | `source_ip.go` | 来源 IP 白名单（netutil） |
| **transport** | `frame.go` | 帧编解码 |
| | `transport.go` | Conn、Dial、ListenTLS |
| | `reconnect.go` | 客户端自动重连 |
| **sessionmgr** | `manager.go` | 在线会话、踢线、路由 |
| | `conn.go` | PacketConn 窄接口 |
| **netstack** | `route_*.go`, `dns_*.go`, `killswitch_*.go` | 平台路由/DNS/杀开关（子进程经 platform） |
| **netutil** | `addr.go`, `cidr.go`, `gateway.go` | HostFromAddr、SplitCIDR、NormalizeIPv4 |
| **config** | `config.go` | LoadClient/LoadServer、YAML 读写 |
| | `client.go`, `server.go` | ApplyDefaults、Validate、PreferGateway |
| | `client_paths.go` | 客户端配置路径解析（exe/GUI/服务一致） |
| **security** | `tls_client.go`, `cert.go` | TLS 构建、证书加载 |
| | `keyenc.go`, `tls_policy.go` | 私钥 AES 密封、指纹策略 |
| **persist** | `store.go` | users/audit/session_stats CRUD |
| | `query_ext.go` | ListUsersPage 等分页查询 |
| | `scan.go`, `jsoncol.go`, `timefmt.go` | 行扫描与列辅助 |
| **auth** | `user.go` | Web 登录、Session Cookie、限流 |
| **ippool** | `pool.go` | VPN IPv4 池分配/释放 |
| **health** | `health.go` | 启动自检与就绪探针 |
| **logstore** | `logstore.go` | 结构化历史日志 SQLite |
| **audit** | `audit.go` | 管理操作审计写入 |
| **logger** | `logger.go` | 分级日志、live.log、滚动 |
| **crypto** | `wg_crypto.go` | X25519 + ChaCha20 隧道加解密 |
| **credentials** | `windows.go` | DPAPI LocalMachine 凭据 |
| **platform** | `elevate_windows.go`, `cmd_windows.go` | UAC 提权、无窗口 route/netsh |
| **singleinstance** | `lock.go` | 客户端单实例文件锁 |
| **brand** | `brand.go` | 产品名、TUN 名、环境变量 |
| **version** | `version.go` | `-version` 与构建注入 |

---

## 依赖规则

1. **`netstack` 不 import `tun`**：网卡索引经 `winnet` 解析；子进程经 `platform.Command`。
2. **`tunnel` 可 import `tun`**：`ServerHandler.TunDev` 为 `tun.Device` 接口。
3. **`api` 不 import `ippool`**：账号/IP 经 `vpnaccount.Service`（含 `DeleteAccount`）；测试 testutil 除外。
4. **数据保留在 `maintenance`**：不由 api handler 内嵌 ticker；`serverapp` 启动 `StartRetentionLoop`。
5. **分页参数在 `paginate`**：api、persist、logstore 共用 `ClampLimit`/`ClampOffset`。
6. **`cmd/*` 保持薄**：`cfg.Log.InitGlobal()`、`security.BuildClientTLS`、`serverapp.Engine.Run()`。
7. **默认值单一来源**：`config.ClientConfig.ApplyDefaults()`、`netutil` 常量、`transport.FromClientConfig`。
8. **禁止薄 re-export**：直接 import `netutil`/`paginate`，不在 api 再包装。
9. **CIDR/地址纯函数**：仅在 `netutil`；`netstack` 只做平台 shell/路由命令。
10. **HTTP 形状 DTO**：在 `readmodel`；`persist` 只负责 SQL（辅助函数在 scan/jsoncol/timefmt）。

---

## HTTP API 路由表

注册于 `internal/api/handler.go` `routes()`；写操作（POST/PUT/PATCH/DELETE）须 Session + CSRF。

| 方法 | 路径 | Handler | 鉴权 |
|------|------|---------|------|
| POST | `/api/v1/login` | handleLogin | 公开（无 CSRF） |
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

**WebUI 页面**（`requireAuthPage`）：`/`, `/users`, `/connections`, `/audit`, `/tools`；`/peers` → 重定向 `/users`；`/login` 公开。

**子路径**（`/api/v1/users/{id}/`）：`export.zip`、`export`、`kick`（POST）、`vpn`（PATCH）、DELETE、POST enable/disable。

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

| 区域 | 测试文件 |
|------|----------|
| netutil | `internal/netutil/*_test.go` |
| config 默认值 | `internal/config/config_test.go` |
| transport 映射 | `internal/transport/config_from_test.go` |
| paginate | `internal/paginate/clamp_test.go` |
| 账号/IP/删号 | `internal/vpnaccount/provision_test.go`, `delete_test.go`, `internal/api/manual_ip_test.go` |
| 全量 | `go test ./...` |

---

## 其他目录

| 目录 | 说明 |
|------|------|
| `internal/` | 包索引与「改 X 来哪」；见 [internal/README.md](../internal/README.md) |
| `web/` | `go:embed` WebUI 模板与静态资源；见 [web/README.md](../web/README.md) |
| `scripts/` | build-local、build-release、验收脚本 |
| `config/` | 参考 YAML 示例（运行时由首次启动生成） |
| `docs/` | 开发/部署/架构文档 |
