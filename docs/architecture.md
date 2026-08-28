# HaoVPN 架构与 CODEMAP

本文是重构后的**包导航单一来源**：分层、依赖规则、改代码去哪找。

> 架构解耦第七轮（2026-08-28）：`fileutil.WriteFileAtomic` / `timeutil`、导出 YAML 迁 `config`、胖文件同包拆分、文档对齐 `clientgui`。

---

## 分层

```
cmd/client, cmd/client-gui, cmd/server   # 入口：flag、单实例、提权（GUI UI 在 clientgui）
        │
        ▼
clientapp / clientgui / serverapp        # 应用编排与桌面 UI
        │
        ├── api ──► vpnaccount           # HTTP 管理 vs 账号/IP 领域（api 不 import ippool）
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
| 新增管理 API | `internal/api/users.go` + `handler.go` routes；业务 `vpnaccount/` |
| API JSON 错误 / `?since=` | `internal/api/httputil.go`（`writeAPIError`、`parseSinceQuery`） |
| 删 VPN 账号（踢线+释 IP） | `internal/vpnaccount/delete.go` |
| 分页 limit/offset | `internal/paginate/clamp.go` |
| 审计/连接事件/日志保留 | `internal/maintenance/retention.go`；默认天数 `config.DefaultRetentionDays` |
| 改握手/策略下发 | `internal/tunnel/handshake.go`, `server_handler.go` |
| 客户端拨号/重连 | `internal/clientapp/engine.go`, `runtime.go` |
| 桌面 GUI（Fyne） | `internal/clientgui/`（入口 `cmd/client-gui` 仅 flag/UAC/主题） |
| 服务端启动流程 | `internal/serverapp/engine.go` |
| YAML 默认值/校验 | `internal/config/client.go`、`server.go` |
| 导出客户端 YAML | `internal/config/client_export.go`（api/export 薄封装） |
| GUI 写回 client.yaml | `internal/config/client_yaml_patch.go`（`SaveClient`） |
| CIDR/地址/IPv4 工具 | `internal/netutil/` |
| SQLite 时间文本 | `internal/timeutil/`（persist/logstore 共用） |
| 敏感文件原子写 / exe 目录 | `internal/fileutil/WriteFileAtomic`、`ExecutableDir` |
| Web/API 读模型 | `internal/readmodel/` |
| Windows 路由/DNS/杀开关 | `internal/netstack/` + `internal/winnet/` |
| 无窗口 route/netsh 子进程 | `internal/platform/`（`CommandOutputError`） |
| 客户端单实例 | `internal/singleinstance/`（127.0.0.1 TCP 协调） |
| TUN 设备 | `internal/tun/` |
| TLS / 数据密钥 | `internal/security/` |
| WebUI 静态资源 | `web/embed.go` |
| 包索引 | [internal/README.md](../internal/README.md) |

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
| **clientapp** | CLI/GUI 共用拨号引擎 | `engine.go`, `runtime.go` | config, transport, tunnel, netstack |
| **clientgui** | Fyne 桌面 UI | `run.go`, `login.go`, `app.go`, `tray.go`, `notice.go` | clientapp, config, singleinstance |
| **serverapp** | 服务端启动编排 | `engine.go` | api, tunnel, tun, netstack, vpnaccount, maintenance |
| **api** | HTTP 管理 API + WebUI | `handler.go`, `users.go`, `httputil.go`, `export.go` | auth, vpnaccount, persist, readmodel, config |
| **readmodel** | Web/API 读模型 DTO | `types.go`, `monitor.go` | — |
| **paginate** | 分页参数规范化 | `clamp.go` | — |
| **maintenance** | 数据保留后台 | `retention.go` | persist, logstore, config |
| **fileutil** | 父目录 / 原子写 / exe 目录 | `mkdir.go`, `atomic.go`, `exe.go` | — |
| **timeutil** | SQLite UTC 时间文本 | `sqlite.go` | — |
| **vpnaccount** | IP 模式、开户、删号 | `service.go`, `provision.go`, `delete.go` | ippool, persist, netutil |
| **tunnel** | 握手协议 | `handshake.go`, `server_handler.go` | transport, crypto, netutil, **tun** |
| **transport** | TLS-TCP 帧、重连 | `transport.go`, `frame.go`, `reconnect.go` | netutil, config |
| **sessionmgr** | 会话与报文路由 | `manager.go`, `register.go`, `kick.go`, `route.go`, `stats.go` | crypto, netutil, persist |
| **netstack** | 路由/DNS/杀开关/NAT | `route_*.go`, `dns_*.go` | winnet, netutil, **platform** |
| **tun** | TUN 抽象 | `tun.go`, `tun_windows.go` | winnet, wintundll, fileutil |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` | platform |
| **netutil** | CIDR/地址/监听/MTU | `cidr.go`, `addr.go`, `gateway.go`, `constants.go` | — |
| **config** | YAML 加载/导出/默认值 | `config.go`, `client_export.go`, `client_yaml_patch.go`, `retention.go` | netutil, fileutil, brand |
| **security** | TLS、密钥加密、绑定自检 | `tls_client.go`, `datakey.go`, `keyenc.go` | netutil, fileutil |
| **persist** | SQLite | `store.go`, `users.go`, `audit_store.go`, `session_store.go`, `query_ext.go` | paginate, readmodel, timeutil |
| **auth** | Web Session + 隧道密码校验 | `service.go`, `login.go`, `tunnel_login.go`, `session.go`, `lockout.go` | persist |
| **ippool** | VPN IP 池 | `pool.go` | — |
| **health** | 启动自检 | `health.go` | config, persist |
| **logstore** | 结构化历史日志库 | `logstore.go` | paginate, timeutil |
| **audit** | 管理审计 | `audit.go` | persist |
| **logger** | 分级日志 | `logger.go` | — |
| **safeutil** | GoSafe、Ticker、Shutdown | `goroutine.go`, `ticker.go` | — |
| **crypto** | 隧道加解密 | `wg_crypto.go` | — |
| **credentials** | Windows DPAPI 凭据 | `windows.go` | fileutil |
| **platform** | UAC、无窗口子进程、错误包装 | `elevate_windows.go`, `cmderr.go` | — |
| **singleinstance** | 客户端单实例（TCP 协调） | `lock.go` | — |
| **brand** | 产品名/路径常量 | `brand.go` | — |
| **version** | 构建版本信息 | `version.go` | — |

每个包均有中文 `doc.go` 说明上下游与关键文件。

---

## 依赖规则

1. **`netstack` 不 import `tun`**：网卡索引经 `winnet`；子进程经 `platform.Command`。
2. **`tunnel` 可 import `tun`**：`ServerHandler.TunDev` 为 `tun.Device`。
3. **`api` 不 import `ippool`**：经 `vpnaccount.Service`；测试 testutil 除外。
4. **数据保留在 `maintenance`**：`serverapp` 启动 `StartRetentionLoop`。
5. **分页在 `paginate`**：api、persist、logstore 共用。
6. **`cmd/*` 保持薄**：逻辑在 `clientapp` / `clientgui` / `serverapp`。
7. **默认值**：`config.ApplyDefaults`；传输秒级常量在 `netutil`；保留天数在 `config.DefaultRetentionDays`。
8. **禁止薄 re-export**：直接 import 叶子包。
9. **CIDR/地址纯函数**：仅在 `netutil`。
10. **HTTP DTO**：在 `readmodel`。
11. **敏感写盘**：配置/凭据/数据密钥/隧道私钥走 `fileutil.WriteFileAtomic`。
12. **SQLite 时间文本**：统一 `timeutil`；`logstore` 不 import `persist`。
13. **客户端导出 YAML**：生成逻辑在 `config.BuildClientExportYAML`；api 只做 HTTP/ZIP。

---

## HTTP API 路由表

注册于 `internal/api/handler.go` `routes()`；写操作须 Session + CSRF。

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

**WebUI**：`/`, `/users`, `/connections`, `/audit`, `/tools`；`/peers` → `/users`；`/login` 公开。

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
| fileutil / timeutil | `internal/fileutil/*_test.go`, `internal/timeutil/*_test.go` |
| config 导出 YAML | `internal/config/client_export_test.go` |
| api 导出兼容 | `internal/api/export_test.go` |
| auth / persist / sessionmgr | 各包 `*_test.go`（拆文件后同包符号不变） |
| 全量 | `go test ./...`；本机构建 `.\scripts\build-local.ps1` |

---

## 其他目录

| 目录 | 说明 |
|------|------|
| `internal/` | 包索引；见 [internal/README.md](../internal/README.md) |
| `web/` | WebUI embed；见 [web/README.md](../web/README.md) |
| `scripts/` | build-local、build-release、验收 |
| `docs/` | 开发/部署/架构文档 |
